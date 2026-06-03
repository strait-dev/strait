import { type Filter, Filters } from "@strait/ui/components/filters";

type FacetedStatusFilterOption = {
  label: string;
  value: string;
};

type FacetedStatusFilterProps = {
  field?: string;
  label?: string;
  onChange: (values: string[]) => void;
  options: FacetedStatusFilterOption[];
  values: string[];
};

const FILTER_ID = "status-filter";

export function FacetedStatusFilter({
  field = "status",
  label = "Status",
  onChange,
  options,
  values,
}: FacetedStatusFilterProps) {
  const filters: Filter[] =
    values.length > 0
      ? [
          {
            field,
            id: `${field}-${FILTER_ID}`,
            operator: "is_any_of",
            values,
          },
        ]
      : [];

  return (
    <Filters
      allowMultiple={false}
      fields={[
        {
          key: field,
          label,
          options,
          type: "multiselect",
        },
      ]}
      filters={filters}
      i18n={{ addFilter: "Filter" }}
      onChange={(nextFilters) => {
        const statusFilter = nextFilters.find(
          (filter) => filter.field === field
        );
        onChange(statusFilter?.values ?? []);
      }}
    />
  );
}
