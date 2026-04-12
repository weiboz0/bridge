import { Server } from "@hocuspocus/server";
import * as Y from "yjs";
import { loadDocumentState, storeDocumentState } from "./documents";

const server = new Server({
  port: 4000,
  debounce: 30000, // Save to DB every 30 seconds (also saves on disconnect)

  async onAuthenticate({ token, documentName }: { token: string; documentName: string }) {
    if (!token) {
      throw new Error("Authentication required");
    }

    const [userId, role] = token.split(":");
    if (!userId || !role) {
      throw new Error("Invalid token format");
    }

    const parts = documentName.split(":");
    if (parts[0] === "session" && parts[2] === "user") {
      const docOwner = parts[3];
      if (role !== "teacher" && role !== "user" && role !== "parent" && userId !== docOwner) {
        throw new Error("Access denied");
      }
    } else if (parts[0] === "broadcast") {
      // Anyone in the session can read broadcast documents
    } else if (documentName === "noop") {
      // Skip noop documents
    } else {
      throw new Error("Invalid document name format");
    }

    return { userId, role };
  },

  async onLoadDocument({ document, documentName }: { document: Y.Doc; documentName: string }) {
    // Skip broadcast and noop documents
    if (documentName.startsWith("broadcast:") || documentName === "noop") return document;

    try {
      const yjsState = await loadDocumentState(documentName);
      if (yjsState) {
        const update = Buffer.from(yjsState, "base64");
        Y.applyUpdate(document, update);
        console.log(`[hocuspocus] Loaded state for: ${documentName}`);
      }
    } catch (err) {
      console.error(`[hocuspocus] Failed to load state for ${documentName}:`, err);
    }

    return document;
  },

  async onStoreDocument({ document, documentName }: { document: Y.Doc; documentName: string }) {
    // Skip broadcast and noop documents
    if (documentName.startsWith("broadcast:") || documentName === "noop") return;

    try {
      const update = Y.encodeStateAsUpdate(document);
      const yjsState = Buffer.from(update).toString("base64");
      const plainText = document.getText("content").toString();

      await storeDocumentState(documentName, yjsState, plainText);
      console.log(`[hocuspocus] Stored state for: ${documentName} (${plainText.length} chars)`);
    } catch (err) {
      console.error(`[hocuspocus] Failed to store state for ${documentName}:`, err);
    }
  },

  async onConnect({ documentName }: { documentName: string }) {
    console.log(`[hocuspocus] Client connected to: ${documentName}`);
  },

  async onDisconnect({ documentName }: { documentName: string }) {
    console.log(`[hocuspocus] Client disconnected from: ${documentName}`);
  },
});

server.listen().then(() => {
  console.log(`[hocuspocus] WebSocket server running on ws://127.0.0.1:4000`);
});
