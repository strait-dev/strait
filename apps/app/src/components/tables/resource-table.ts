const ACTIONS_COLUMN_ID = "actions";

export const RESOURCE_TABLE_CLASS_NAMES = {
  base: "min-w-full lg:min-w-[1200px]",
} as const;

export const RESOURCE_TABLE_EMPTY_CLASS_NAME =
  "sticky left-0 h-[300px] w-[calc(100vw-2rem)] max-w-full sm:static sm:w-auto";

export function getResourceTableInitialState() {
  return {
    columnPinning: {
      right: [ACTIONS_COLUMN_ID],
    },
  };
}
