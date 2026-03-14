import {
  ArrowDown01Icon,
  Search01Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandList,
} from "@strait/ui/components/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@strait/ui/components/popover";
import { cn } from "@strait/ui/utils/index";
import { useCallback, useState } from "react";
import { CircleFlag } from "react-circle-flags";
import { countries } from "@/utils/data";

export type Country = {
  value: string;
  label: string;
  iso: string;
  symbol: string;
  flag: string;
};

type CountryDropdownProps = {
  onValueChange?: (value: string) => void;
  value?: string;
  disabled?: boolean;
  placeholder?: string;
  className?: string;
  ref?: React.Ref<HTMLButtonElement>;
};

const CountryDropdown = ({
  onValueChange,
  value,
  disabled = false,
  placeholder = "Select a country",
  className,
  ref,
  ...props
}: CountryDropdownProps) => {
  const [open, setOpen] = useState(false);
  const [searchValue, setSearchValue] = useState("");

  const selectedCountry = value
    ? countries.find(
        (country) => country.value === value || country.iso === value
      )
    : undefined;

  const handleSelect = useCallback(
    (country: Country) => {
      onValueChange?.(country.value);
      setOpen(false);
    },
    [onValueChange]
  );

  return (
    <Popover
      onOpenChange={(isOpen) => {
        setOpen(isOpen);
        if (!isOpen) {
          setSearchValue("");
        }
      }}
      open={open}
    >
      <PopoverTrigger
        className={cn(
          "flex h-8 w-full items-center justify-between gap-2 rounded-custom border border-input bg-transparent px-3 py-2 text-foreground text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 data-placeholder:text-muted-foreground dark:aria-invalid:ring-destructive/40 [&>span]:line-clamp-1",
          className
        )}
        disabled={disabled}
        {...props}
      >
        {selectedCountry ? (
          <div className="flex w-0 grow items-center gap-2 overflow-hidden">
            <div className="inline-flex h-4 w-4 shrink-0 items-center justify-center overflow-hidden rounded-full">
              <CircleFlag
                countryCode={selectedCountry.iso.toLowerCase()}
                height={16}
              />
            </div>
            <span className="overflow-hidden text-ellipsis whitespace-nowrap">
              {selectedCountry.label}
            </span>
          </div>
        ) : (
          <span className="text-muted-foreground/70">{placeholder}</span>
        )}
        <HugeiconsIcon
          className="size-4 shrink-0 text-muted-foreground/80"
          icon={ArrowDown01Icon}
        />
      </PopoverTrigger>
      <PopoverContent
        className="w-(--radix-popover-trigger-width) p-0"
        side="bottom"
      >
        <Command>
          <div className="flex items-center border-input border-b px-3 py-2">
            <div className="inline-flex h-4 w-4 shrink-0 items-center justify-center">
              <HugeiconsIcon
                className="text-muted-foreground/80"
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
          <CommandList>
            <CommandEmpty>No country found.</CommandEmpty>
            <CommandGroup className="p-1">
              {countries
                .filter((country) => !!country.label && !!country.iso)
                .filter(
                  (country) =>
                    searchValue === "" ||
                    country.label
                      .toLowerCase()
                      .includes(searchValue.toLowerCase())
                )
                .map((option) => (
                  <CommandItem
                    key={option.iso}
                    onSelect={() => handleSelect(option)}
                  >
                    <div className="inline-flex h-4 w-4 shrink-0 items-center justify-center overflow-hidden rounded-full">
                      <CircleFlag
                        countryCode={option.iso.toLowerCase()}
                        height={16}
                      />
                    </div>
                    <span className="overflow-hidden text-ellipsis whitespace-nowrap">
                      {option.label}
                    </span>
                    <HugeiconsIcon
                      className={cn(
                        "ml-auto h-4 w-4 shrink-0",
                        option.value === selectedCountry?.value
                          ? "opacity-100"
                          : "opacity-0"
                      )}
                      icon={Tick02Icon}
                    />
                  </CommandItem>
                ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
};

CountryDropdown.displayName = "CountryDropdown";

export { CountryDropdown };
