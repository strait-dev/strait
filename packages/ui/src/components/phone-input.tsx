"use client";

import {
  ArrowDown01Icon,
  Search01Icon,
  Tick01Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import React from "react";
import { CircleFlag } from "react-circle-flags";
import RPNInput, {
  type Country,
  getCountryCallingCode,
  type Value,
} from "react-phone-number-input";
import { cn } from "../utils/index";
import { Button } from "./button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandList,
} from "./command";
import { Input } from "./input";
import { Popover, PopoverContent, PopoverTrigger } from "./popover";

type CountryEntry = { label: string; value: Country | undefined };

type CountrySelectProps = {
  disabled?: boolean;
  value: Country;
  options: CountryEntry[];
  onChange: (country: Country) => void;
};

const CountrySelect = ({
  disabled,
  value: selectedCountry,
  options: countryList,
  onChange,
}: CountrySelectProps) => {
  const [searchValue, setSearchValue] = React.useState("");
  const [isOpen, setIsOpen] = React.useState(false);

  return (
    <Popover
      modal
      onOpenChange={(open) => {
        setIsOpen(open);
        if (open) {
          setSearchValue("");
        }
      }}
      open={isOpen}
    >
      <PopoverTrigger
        render={
          <Button
            className="flex gap-1 rounded-s-md rounded-e-none border-r-0 px-3 focus:z-10"
            disabled={disabled}
            type="button"
            variant="outline"
          />
        }
      >
        <FlagComponent
          country={selectedCountry}
          countryName={selectedCountry}
        />
        <HugeiconsIcon
          className={cn(
            "-mr-2 size-4 text-muted-foreground/80",
            disabled ? "hidden" : ""
          )}
          icon={ArrowDown01Icon}
        />
      </PopoverTrigger>
      <PopoverContent className="w-[300px] p-0">
        <Command>
          <div className="flex items-center border-input border-b px-3 py-2">
            <div className="inline-flex h-4 w-4 shrink-0 items-center justify-center">
              <HugeiconsIcon
                className="size-4 text-muted-foreground/80"
                icon={Search01Icon}
              />
            </div>
            <input
              aria-label="Search country"
              className="ml-3 flex h-6 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground/70 disabled:cursor-not-allowed disabled:opacity-50"
              cmdk-input=""
              onChange={(e) => setSearchValue(e.target.value)}
              placeholder="Search country..."
              value={searchValue}
            />
          </div>
          <CommandList className="max-h-72">
            <CommandEmpty>No country found.</CommandEmpty>
            <CommandGroup className="p-1">
              {countryList
                .filter(
                  ({ label }) =>
                    searchValue === "" ||
                    label.toLowerCase().includes(searchValue.toLowerCase())
                )
                .map(({ value, label }) =>
                  value ? (
                    <CountrySelectOption
                      country={value}
                      countryName={label}
                      key={value}
                      onChange={onChange}
                      onSelectComplete={() => setIsOpen(false)}
                      selectedCountry={selectedCountry}
                    />
                  ) : null
                )}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
};

type CountrySelectOptionProps = {
  country: Country;
  countryName: string;
  selectedCountry: Country;
  onChange: (country: Country) => void;
  onSelectComplete: () => void;
};

const CountrySelectOption = ({
  country,
  countryName,
  selectedCountry,
  onChange,
  onSelectComplete,
}: CountrySelectOptionProps) => {
  const handleSelect = () => {
    onChange(country);
    onSelectComplete();
  };

  return (
    <CommandItem className="gap-2" onSelect={handleSelect}>
      <FlagComponent country={country} countryName={countryName} />
      <span className="flex-1 text-sm">{countryName}</span>
      <span className="text-foreground/50 text-sm">{`+${getCountryCallingCode(country)}`}</span>
      <HugeiconsIcon
        className={`ml-auto size-4 ${country === selectedCountry ? "opacity-100" : "opacity-0"}`}
        icon={Tick01Icon}
      />
    </CommandItem>
  );
};

type FlagComponentProps = {
  country: Country | undefined;
  countryName: string;
};

const FlagComponent = ({ country, countryName }: FlagComponentProps) => {
  if (!country) {
    return (
      <span className="inline-flex h-4 w-4 shrink-0 items-center justify-center overflow-hidden rounded-full bg-muted" />
    );
  }

  return (
    <span className="inline-flex h-4 w-4 shrink-0 items-center justify-center overflow-hidden rounded-full">
      <CircleFlag
        countryCode={country.toLowerCase()}
        height={16}
        title={countryName}
      />
    </span>
  );
};

type PhoneInputProps = Omit<
  React.ComponentProps<"input">,
  "onChange" | "value" | "ref"
> &
  Omit<React.ComponentProps<typeof RPNInput>, "onChange"> & {
    onChange?: (value: Value) => void;
    onCountryChange?: (country: Country) => void;
  };

const PhoneInput = ({
  className,
  onChange,
  onCountryChange,
  value,
  ref,
  ...props
}: PhoneInputProps & {
  ref?: React.Ref<React.ComponentRef<typeof RPNInput>>;
}) => {
  return (
    <RPNInput
      className={cn("flex", className)}
      countrySelectComponent={CountrySelect}
      flagComponent={FlagComponent}
      inputComponent={InputComponent}
      onChange={(inputValue: Value | undefined) =>
        onChange?.(inputValue || ("" as Value))
      }
      onCountryChange={onCountryChange}
      ref={ref}
      /**
       * Handles the onChange event.
       *
       * react-phone-number-input might trigger the onChange event as undefined
       * when a valid phone number is not entered. To prevent this,
       * the value is coerced to an empty string.
       *
       * @param {E164Number | undefined} inputValue - The entered value
       */
      smartCaret={false}
      value={value || undefined}
      {...props}
    />
  );
};

PhoneInput.displayName = "PhoneInput";

const InputComponent = ({
  className,
  ref,
  ...props
}: React.ComponentProps<"input"> & {
  ref?: React.RefObject<HTMLInputElement | null>;
}) => (
  <Input
    className={cn("rounded-s-none rounded-e-md", className)}
    {...props}
    ref={ref}
  />
);

InputComponent.displayName = "InputComponent";

export { PhoneInput };
