"use client";

import {
  addDays,
  endOfMonth,
  endOfYear,
  startOfMonth,
  startOfYear,
  subDays,
  subMonths,
  subYears,
} from "date-fns";
import { useCallback, useMemo, useState } from "react";
import type { DateRange } from "react-day-picker";

import { cn } from "../utils/index";
import { Button } from "./button";
import { Calendar } from "./calendar";

const ONE_DAY = 1;
const SEVEN_DAYS = 7;
const THIRTY_DAYS = 30;
const DAYS_IN_YEAR = 365;

type RangePreset = {
  name: string;
  value: DateRange;
};

type RangeCalendarWithPresetsProps = {
  /**
   * The selected date range.
   */
  selected?: DateRange;

  /**
   * The default date range.
   */
  _defaultRange?: DateRange;

  /**
   * Callback fired when the date range changes.
   */
  onSelect?: (range: DateRange | undefined) => void;

  /**
   * The class name for the root element.
   */
  className?: string;

  /**
   * Custom presets to show in the sidebar.
   * If not provided, default presets will be used.
   */
  presets?: RangePreset[];

  /**
   * Whether to disable future dates.
   * @default true
   */
  disableFutureDates?: boolean;

  /**
   * Callback fired when the apply button is clicked.
   */
  onApply?: () => void;

  /**
   * Callback fired when the reset button is clicked.
   */
  onReset?: () => void;

  /**
   * Whether to show the apply and reset buttons.
   * @default false
   */
  showFooter?: boolean;

  /**
   * The number of months to display in the calendar.
   * @default 2
   */
  numberOfMonths?: number;

  /**
   * Whether to show outside days in the calendar.
   * @default true
   */
  showOutsideDays?: boolean;
};

export function RangeCalendarWithPresets({
  selected,
  onSelect,
  className,
  numberOfMonths = 2,
  showOutsideDays = true,
  onApply,
  onReset,
  showFooter = false,
}: RangeCalendarWithPresetsProps) {
  const [_initialState] = useState(selected);

  const today = useMemo(() => new Date(), []);

  const defaultPresets = useMemo(
    () => [
      {
        name: "Hoje",
        value: {
          from: today,
          to: today,
        },
      },
      {
        name: "Ontem",
        value: {
          from: subDays(today, 1),
          to: subDays(today, 1),
        },
      },
      {
        name: "Últimos 7 dias",
        value: {
          from: subDays(today, SEVEN_DAYS),
          to: today,
        },
      },
      {
        name: "Últimos 30 dias",
        value: {
          from: subDays(today, THIRTY_DAYS),
          to: today,
        },
      },
      {
        name: "Mês atual",
        value: {
          from: startOfMonth(today),
          to: today,
        },
      },
      {
        name: "Mês anterior",
        value: {
          from: startOfMonth(subMonths(today, 1)),
          to: endOfMonth(subMonths(today, 1)),
        },
      },
      {
        name: "Ano atual",
        value: {
          from: startOfYear(today),
          to: today,
        },
      },
      {
        name: "Ano anterior",
        value: {
          from: startOfYear(subYears(today, 1)),
          to: endOfYear(subYears(today, 1)),
        },
      },
    ],
    [today]
  );

  const currentPresets = useMemo(() => defaultPresets, [defaultPresets]);

  const handlePresetClick = useCallback(
    (preset: RangePreset) => {
      if (onSelect) {
        onSelect(preset.value);
      }
    },
    [onSelect]
  );

  const disabledDates = useMemo(
    () => [
      {
        from: addDays(today, ONE_DAY),
        to: addDays(today, DAYS_IN_YEAR),
      },
    ],
    [today]
  );

  const isSelectedPreset = useCallback(
    (preset: RangePreset) => {
      if (
        !(
          selected?.from &&
          selected?.to &&
          preset.value.from &&
          preset.value.to
        )
      ) {
        return false;
      }

      // Compare dates ignoring time
      const selectedFromDate = new Date(selected.from.setHours(0, 0, 0, 0));
      const selectedToDate = new Date(selected.to.setHours(0, 0, 0, 0));
      const presetFromDate = new Date(preset.value.from.setHours(0, 0, 0, 0));
      const presetToDate = new Date(preset.value.to.setHours(0, 0, 0, 0));

      return (
        selectedFromDate.getTime() === presetFromDate.getTime() &&
        selectedToDate.getTime() === presetToDate.getTime()
      );
    },
    [selected]
  );

  return (
    <div
      className={cn(
        "flex flex-col gap-4 rounded-md border bg-popover p-4 text-popover-foreground shadow-md",
        className
      )}
    >
      <div className="grid grid-cols-4 gap-2">
        {currentPresets.map((preset) => (
          <Button
            className={cn(
              selected && isSelectedPreset(preset)
                ? "bg-accent text-accent-foreground"
                : null
            )}
            key={preset.name}
            onClick={() => handlePresetClick(preset)}
            size="sm"
            variant="outline"
          >
            {preset.name}
          </Button>
        ))}
      </div>
      <div className="flex flex-col">
        <div className="relative">
          <Calendar
            disabled={disabledDates}
            mode="range"
            numberOfMonths={numberOfMonths}
            onSelect={onSelect}
            selected={selected}
            showOutsideDays={showOutsideDays}
          />
        </div>
        {showFooter ? (
          <div className="flex items-center justify-end gap-2 border-t p-3">
            <Button onClick={onReset} variant="outline">
              Limpar
            </Button>
            <Button
              disabled={!(Boolean(selected?.from) && Boolean(selected?.to))}
              onClick={onApply}
            >
              Aplicar
            </Button>
          </div>
        ) : null}
      </div>
    </div>
  );
}
