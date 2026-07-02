import {
  type RowData,
  type TableOptions,
  useReactTable,
} from "@tanstack/react-table";

export function useAppReactTable<TData extends RowData>(
  options: TableOptions<TData>
) {
  // react-doctor-disable-next-line react-hooks-js/incompatible-library -- TanStack Table owns mutable table internals by design.
  return useReactTable(options);
}
