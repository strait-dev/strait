"use client";

import { Calendar03Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { type CalendarDate, parseDate } from "@internationalized/date";
import { useOverlayTrigger } from "@react-aria/overlays";
import { useOverlayTriggerState } from "@react-stately/overlays";
import type { RangeValue } from "@react-types/shared";
import { format, startOfDay } from "date-fns";
import { useCallback, useEffect, useState } from "react";
import {
  Button as AriaButton,
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
import { RangeCalendarWithPresets } from "./range-calendar-with-presets";

type DateRangePickerWithPresetsProps = {
  /**
   * The current value of the date range picker.
   * @type DateRange | undefined
   */
  value?: DateRange;

  /**
   * The default date range.
   * @type DateRange
   */
  defaultValue?: DateRange;

  /**
   * Handler that is called when the value changes.
   * @type (value: DateRange | undefined) => void
   */
  onChange?: (value: DateRange | undefined) => void;

  /**
   * The label for the date range picker.
   * @type string | undefined
   */
  label?: string;

  /**
   * Additional CSS class name for the container.
   * @type string | undefined
   */
  className?: string;
};

export function DateRangePickerWithPresets({
  value,
  defaultValue,
  onChange,
  label,
  className,
}: DateRangePickerWithPresetsProps) {
  const [tempDateRange, setTempDateRange] = useState<DateRange>(
    value ?? defaultValue ?? { from: undefined, to: undefined }
  );

  // Update temp value when external value changes
  useEffect(() => {
    if (value) {
      setTempDateRange(value);
    }
  }, [value]);

  const overlayState = useOverlayTriggerState({});
  const { triggerProps, overlayProps } = useOverlayTrigger(
    { type: "dialog" },
    overlayState
  );

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
    if (!newValue?.start) {
      setTempDateRange(defaultValue ?? { from: undefined, to: undefined });
      return;
    }

    const from = new Date(newValue.start.toString());
    const to = newValue.end ? new Date(newValue.end.toString()) : undefined;

    setTempDateRange({ from, to: to ?? from });
  };

  const handleTempChange = useCallback(
    (range: DateRange | undefined) => {
      if (!range) {
        setTempDateRange(defaultValue ?? { from: undefined, to: undefined });
        return;
      }
      setTempDateRange(range);
    },
    [defaultValue]
  );

  const handleApply = useCallback(() => {
    if (onChange && tempDateRange.from && tempDateRange.to) {
      // Convert Date objects to ensure consistent format for router
      const from = startOfDay(tempDateRange.from);
      const to = startOfDay(tempDateRange.to);

      const formattedDateRange = {
        from,
        to,
      };
      onChange(formattedDateRange);
    }
    overlayState.close();
  }, [onChange, overlayState, tempDateRange]);

  const handleReset = useCallback(() => {
    const resetValue = defaultValue ?? { from: undefined, to: undefined };
    setTempDateRange(resetValue);
    if (onChange) {
      onChange(resetValue);
    }
    overlayState.close();
  }, [onChange, overlayState, defaultValue]);

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
          <AriaButton
            {...triggerProps}
            className="mr-2 flex h-8 w-8 items-center justify-center rounded-md transition-colors hover:bg-muted"
          >
            <HugeiconsIcon className="size-4" icon={Calendar03Icon} />
          </AriaButton>
        </Group>
        <Popover
          {...overlayProps}
          className={cn(
            "z-50 mt-2 w-auto rounded-md shadow-md outline-hidden",
            "data-entering:animate-in data-exiting:animate-out",
            "data-entering:fade-in-0 data-exiting:fade-out-0",
            "data-entering:zoom-in-95 data-exiting:zoom-out-95",
            "data-[placement=bottom]:slide-in-from-top-2",
            "data-[placement=left]:slide-in-from-right-2",
            "data-[placement=right]:slide-in-from-left-2",
            "data-[placement=top]:slide-in-from-bottom-2"
          )}
          isOpen={overlayState.isOpen}
          offset={4}
          onOpenChange={overlayState.setOpen}
        >
          <Dialog className="outline-hidden">
            <RangeCalendarWithPresets
              onApply={handleApply}
              onReset={handleReset}
              onSelect={handleTempChange}
              selected={tempDateRange}
              showFooter
            />
          </Dialog>
        </Popover>
      </RACDateRangePicker>
    </I18nProvider>
  );
}
