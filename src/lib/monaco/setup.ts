import { loader } from "@monaco-editor/react";
import type * as monacoTypes from "monaco-editor";

let initialized = false;

export function setupMonaco() {
  if (initialized) return;
  initialized = true;

  loader.init().then((monaco) => {
    registerPythonCompletions(monaco);
  }).catch((error) => {
    console.error("Failed to initialize Monaco editor:", error);
  });
}

function registerPythonCompletions(monaco: typeof monacoTypes) {
  monaco.languages.registerCompletionItemProvider("python", {
    provideCompletionItems(model, position) {
      const word = model.getWordUntilPosition(position);
      const range = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn,
      };

      const keywords = [
        "False", "None", "True", "and", "as", "assert", "async", "await",
        "break", "class", "continue", "def", "del", "elif", "else",
        "except", "finally", "for", "from", "global", "if", "import",
        "in", "is", "lambda", "nonlocal", "not", "or", "pass", "raise",
        "return", "try", "while", "with", "yield",
      ];

      const builtins = [
        "print", "input", "len", "range", "int", "float", "str", "list",
        "dict", "set", "tuple", "bool", "type", "isinstance", "enumerate",
        "zip", "map", "filter", "sorted", "reversed", "abs", "max", "min",
        "sum", "round", "open", "super", "property", "staticmethod",
        "classmethod", "hasattr", "getattr", "setattr",
      ];

      const suggestions: monacoTypes.languages.CompletionItem[] = [
        ...keywords.map((kw) => ({
          label: kw,
          kind: monaco.languages.CompletionItemKind.Keyword,
          insertText: kw,
          range,
        })),
        ...builtins.map((fn) => ({
          label: fn,
          kind: monaco.languages.CompletionItemKind.Function,
          insertText: fn,
          range,
        })),
      ];

      return { suggestions };
    },
  });
}
