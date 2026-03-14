"use client";

import { Calendar03Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { type CalendarDate, parseDate } from "@internationalized/date";
import type { RangeValue } from "@react-types/shared";
import { format } from "date-fns";
import {
  Button,
  DateInput,
  DateSegment,
  Dialog,
  Group,
  I18nProvider,
  Label,
  Popover,
  DateRangePicker as RACDateRangePicker,
} from "react-aria-components";
import type { DateRange } from "react-day-picker";

import { cn } from "../utils/index";
import { Calendar } from "./calendar";

type DateRangePickerProps = {
  /**
   * The selected date range.
   * @type DateRange | undefined
   */
  value?: DateRange;

  /**
   * Callback fired when the date range changes.
   * @param value The new date range value
   */
  onChange?: (value: DateRange | undefined) => void;

  /**
   * The label text for the date range picker.
   * @type string | undefined
   */
  label?: string;

  /**
   * The class name for the root element.
   * @type string
   */
  className?: string;

  /**
   * Indicates if the input has an error.
   * @type boolean
   */
  error?: boolean;

  /**
   * The error message to display when the input has an error.
   * @type string | undefined
   */
  errorMessage?: string;

  /**
   * Indicates if the input is disabled.
   * @type boolean
   */
  disabled?: boolean;
};

export function DateRangePicker({
  value,
  onChange,
  label,
  className,
}: DateRangePickerProps) {
  // Convert DateRange to React Aria's RangeValue<DateValue>
  const dateValue: RangeValue<CalendarDate> | null = (() => {
    if (value?.from && value?.to) {
      return {
        start: parseDate(format(value.from, "yyyy-MM-dd")),
        end: parseDate(format(value.to, "yyyy-MM-dd")),
      };
    }
    if (value?.from) {
      return {
        start: parseDate(format(value.from, "yyyy-MM-dd")),
        end: parseDate(format(value.from, "yyyy-MM-dd")),
      };
    }
    return null;
  })();

  // Convert React Aria's RangeValue<DateValue> back to DateRange
  const handleChange = (newValue: RangeValue<CalendarDate> | null) => {
    if (!onChange) {
      return;
    }

    if (!newValue?.start) {
      onChange(undefined);
      return;
    }

    const from = new Date(newValue.start.toString());
    const to = newValue.end ? new Date(newValue.end.toString()) : undefined;

    onChange({ from, to });
  };

  return (
    <I18nProvider locale="pt-BR">
      <RACDateRangePicker
        className={cn("flex flex-col gap-2", className)}
        onChange={handleChange}
        value={dateValue}
      >
        {label ? (
          <Label className="font-medium text-foreground text-sm">{label}</Label>
        ) : null}
        <Group
          className={cn(
            "flex h-9 w-full items-center rounded-md border border-input bg-background text-sm ring-offset-background",
            "focus-within:outline-hidden focus-within:ring-[3px] focus-within:ring-ring/50 focus-within:ring-offset-2",
            "disabled:cursor-not-allowed disabled:opacity-50"
          )}
        >
          <DateInput className="flex flex-1 px-2" slot="start">
            {(segment) => (
              <DateSegment
                className={cn(
                  "focus:rounded-md focus:bg-accent focus:text-accent-foreground focus:outline-hidden",
                  "placeholder:text-muted-foreground"
                )}
                segment={segment}
              />
            )}
          </DateInput>
          <span aria-hidden="true" className="px-2 text-muted-foreground">
            to
          </span>
          <DateInput className="flex flex-1 px-2" slot="end">
            {(segment) => (
              <DateSegment
                className={cn(
                  "focus:rounded-md focus:bg-accent focus:text-accent-foreground focus:outline-hidden",
                  "placeholder:text-muted-foreground"
                )}
                segment={segment}
              />
            )}
          </DateInput>
          <Button className="mr-2 flex h-8 w-8 items-center justify-center rounded-md transition-colors hover:bg-muted">
            <HugeiconsIcon className="size-4" icon={Calendar03Icon} />
          </Button>
        </Group>
        <Popover
          className={cn(
            "z-50 w-auto rounded-md border border-border bg-background p-2 shadow-md outline-hidden",
            "data-entering:animate-in data-exiting:animate-out",
            "data-entering:fade-in-0 data-exiting:fade-out-0",
            "data-entering:zoom-in-95 data-exiting:zoom-out-95",
            "data-[placement=bottom]:slide-in-from-top-2",
            "data-[placement=left]:slide-in-from-right-2",
            "data-[placement=right]:slide-in-from-left-2",
            "data-[placement=top]:slide-in-from-bottom-2"
          )}
          offset={4}
        >
          <Dialog className="p-0 outline-hidden">
            <Calendar
              className="border-0"
              defaultMonth={value?.from}
              mode="range"
              numberOfMonths={2}
              onSelect={onChange}
              selected={value}
            />
          </Dialog>
        </Popover>
      </RACDateRangePicker>
    </I18nProvider>
  );
}
