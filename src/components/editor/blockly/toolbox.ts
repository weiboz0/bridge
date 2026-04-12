/**
 * K-5 Blockly toolbox configuration.
 * Simplified categories for elementary students.
 */
export const k5Toolbox = {
  kind: "categoryToolbox",
  contents: [
    {
      kind: "category",
      name: "Logic",
      colour: "#5b80a5",
      contents: [
        { kind: "block", type: "controls_if" },
        { kind: "block", type: "logic_compare" },
        { kind: "block", type: "logic_operation" },
        { kind: "block", type: "logic_negate" },
        { kind: "block", type: "logic_boolean" },
      ],
    },
    {
      kind: "category",
      name: "Loops",
      colour: "#5ba55b",
      contents: [
        { kind: "block", type: "controls_repeat_ext" },
        { kind: "block", type: "controls_whileUntil" },
        { kind: "block", type: "controls_for" },
        { kind: "block", type: "controls_forEach" },
      ],
    },
    {
      kind: "category",
      name: "Math",
      colour: "#5b67a5",
      contents: [
        { kind: "block", type: "math_number" },
        { kind: "block", type: "math_arithmetic" },
        { kind: "block", type: "math_modulo" },
        { kind: "block", type: "math_random_int" },
      ],
    },
    {
      kind: "category",
      name: "Text",
      colour: "#5ba58c",
      contents: [
        { kind: "block", type: "text" },
        { kind: "block", type: "text_join" },
        { kind: "block", type: "text_length" },
        { kind: "block", type: "text_print" },
        { kind: "block", type: "text_prompt_ext" },
      ],
    },
    {
      kind: "category",
      name: "Variables",
      colour: "#a55b80",
      custom: "VARIABLE",
    },
    {
      kind: "category",
      name: "Functions",
      colour: "#995ba5",
      custom: "PROCEDURE",
    },
  ],
};
