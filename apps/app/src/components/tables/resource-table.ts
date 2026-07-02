const ACTIONS_COLUMN_ID = "actions";

export const RESOURCE_TABLE_CLASS_NAMES = {
  base: "min-w-[1200px]",
} as const;

export function getResourceTableInitialState() {
  return {
    columnPinning: {
      right: [ACTIONS_COLUMN_ID],
    },
  };
}
