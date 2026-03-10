export type Option = {
  label: string;
  value: string;
  icon?: React.ComponentType<{ className?: string }>;
  count?: number;
};

export type StringKeyOf<TData> = Extract<keyof TData, string>;

export type DataTableFilterField<TData> = {
  id: StringKeyOf<TData>;
  label: string;
  value: keyof TData;
  placeholder?: string;
  options?: Option[];
};
