"use client";

import { Calendar03Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { format, setDefaultOptions } from "date-fns";
import { ptBR } from "date-fns/locale";
import { useCallback, useMemo, useState } from "react";
import type { DropdownNavProps, DropdownProps } from "react-day-picker";

import { cn } from "../utils/index";
import { Button } from "./button";
import { Calendar } from "./calendar";
import { Popover, PopoverContent, PopoverTrigger } from "./popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./select";

// Set date-fns to preserve the date values without timezone conversion
setDefaultOptions({
  // This ensures that date-fns doesn't apply timezone conversions
  // when formatting or parsing dates
  weekStartsOn: 0,
});

// Capitalized month names for pt-BR
const capitalizedMonths = {
  janeiro: "Janeiro",
  fevereiro: "Fevereiro",
  março: "Março",
  abril: "Abril",
  maio: "Maio",
  junho: "Junho",
  julho: "Julho",
  agosto: "Agosto",
  setembro: "Setembro",
  outubro: "Outubro",
  novembro: "Novembro",
  dezembro: "Dezembro",
};

// Calendar dropdown navigation component
function DropdownNav(props: DropdownNavProps) {
  return (
    <div className="flex w-full items-center gap-2" data-slot="dropdown-nav">
      {props.children}
    </div>
  );
}

// Calendar dropdown component
function CalendarDropdown({
  props,
  handleCalendarChange,
}: {
  props: DropdownProps;
  handleCalendarChange: (
    value: string | number,
    e: React.ChangeEventHandler<HTMLSelectElement>
  ) => void;
}) {
  return (
    <Select
      data-slot="dropdown"
      onValueChange={(value) => {
        if (props.onChange && value !== null) {
          handleCalendarChange(value, props.onChange);
        }
      }}
      value={String(props.value)}
    >
      <SelectTrigger
        className="h-9 w-fit font-medium first:grow"
        data-slot="dropdown-trigger"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent
        className="max-h-[min(26rem,var(--radix-select-content-available-height))]"
        data-slot="dropdown-content"
      >
        {props.options?.map((option) => {
          // Capitalize month names if this is a month dropdown
          let label = option.label;
          if (
            typeof label === "string" &&
            Object.keys(capitalizedMonths).includes(label.toLowerCase())
          ) {
            label =
              capitalizedMonths[
                label.toLowerCase() as keyof typeof capitalizedMonths
              ];
          }

          return (
            <SelectItem
              data-slot="dropdown-item"
              disabled={option.disabled}
              key={option.value}
              value={String(option.value)}
            >
              {label}
            </SelectItem>
          );
        })}
      </SelectContent>
    </Select>
  );
}

// Dropdown wrapper component to avoid inline component definition
function DropdownWrapper({
  handleCalendarChange,
}: {
  handleCalendarChange: (
    value: string | number,
    e: React.ChangeEventHandler<HTMLSelectElement>
  ) => void;
}) {
  return function Dropdown(props: DropdownProps) {
    return (
      <CalendarDropdown
        handleCalendarChange={handleCalendarChange}
        props={props}
      />
    );
  };
}

type DatePickerWithMonthYearProps = {
  /** The current value (controlled). */
  value?: Date;
  /** Handler that is called when the value changes. */
  onChange?: (value: Date | undefined) => void;
  /** The default value (uncontrolled). */
  defaultValue?: Date;
  /** Whether the date picker is disabled. */
  disabled?: boolean;
  /** Additional CSS class name for the container. */
  className?: string;
  /** Error state for validation styling. */
  error?: boolean;
  /** Minimum selectable date. */
  minValue?: Date;
  /** Maximum selectable date. */
  maxValue?: Date;
  /** Placeholder text when no date is selected */
  placeholder?: string;
};

function DatePickerWithMonthYear({
  value,
  onChange,
  defaultValue,
  disabled,
  className,
  error,
  minValue,
  maxValue,
  placeholder = "Selecione uma data",
}: DatePickerWithMonthYearProps) {
  // Use internal state if not controlled
  const [internalDate, setInternalDate] = useState<Date | undefined>(
    defaultValue
  );

  // Use either controlled value or internal state
  const selectedDate = value === undefined ? internalDate : value;

  // Format date for display
  const formattedDate = useMemo(() => {
    if (!selectedDate) {
      return "";
    }
    return format(selectedDate, "dd/MM/yyyy", { locale: ptBR });
  }, [selectedDate]);

  // Handle calendar dropdown change
  const handleCalendarChange = useCallback(
    (
      _value: string | number,
      _e: React.ChangeEventHandler<HTMLSelectElement>
    ) => {
      const _event = {
        target: {
          value: String(_value),
        },
      } as React.ChangeEvent<HTMLSelectElement>;
      _e(_event);
    },
    []
  );

  // Handle date selection
  const handleSelect = useCallback(
    (date: Date | undefined) => {
      if (!date) {
        if (value === undefined) {
          setInternalDate(undefined);
        }
        if (onChange) {
          onChange(undefined);
        }
        return;
      }

      // Create a new date with the same year, month, and day
      // This ensures we're using the date as displayed in the UI
      // and prevents timezone issues
      const newDate = new Date(
        date.getFullYear(),
        date.getMonth(),
        date.getDate(),
        0,
        0,
        0,
        0
      );

      if (value === undefined) {
        setInternalDate(newDate);
      }

      if (onChange) {
        onChange(newDate);
      }
    },
    [onChange, value]
  );

  // Calendar components for month/year dropdowns
  const calendarComponents = useMemo(
    () => ({
      DropdownNav,
      Dropdown: DropdownWrapper({ handleCalendarChange }),
    }),
    [handleCalendarChange]
  );

  return (
    <div
      className={cn("relative", className)}
      data-slot="date-picker-with-month-year"
    >
      <Popover>
        <PopoverTrigger
          render={
            <Button
              className={cn(
                "w-full justify-start text-left font-normal",
                !selectedDate && "text-muted-foreground",
                error ? "border-destructive" : null,
                "h-9"
              )}
              data-slot="date-picker-trigger"
              disabled={disabled}
              variant="outline"
            />
          }
        >
          <HugeiconsIcon className="mr-2 size-4" icon={Calendar03Icon} />
          {formattedDate || placeholder}
        </PopoverTrigger>
        <PopoverContent
          align="start"
          className="w-auto p-3"
          data-slot="date-picker-content"
        >
          <Calendar
            captionLayout="dropdown"
            className="border-0"
            classNames={{
              month_caption: "mx-0",
            }}
            components={calendarComponents}
            data-slot="calendar"
            defaultMonth={selectedDate || new Date()}
            endMonth={maxValue}
            fixedWeeks
            hideNavigation
            locale={ptBR}
            mode="single"
            onSelect={handleSelect}
            selected={selectedDate}
            startMonth={minValue}
          />
        </PopoverContent>
      </Popover>
    </div>
  );
}

export { DatePickerWithMonthYear };
