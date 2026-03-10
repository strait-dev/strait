"use client";

import { Add01Icon, Cancel01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { nanoid } from "nanoid";
import { useId, useRef, useState } from "react";
import { useHotkeys } from "react-hotkeys-hook";
import { Badge } from "./badge";
import { Button } from "./button";
import { Input } from "./input";

export type Tag = {
  id: string;
  text: string;
};

type InputWithInnerTagsProps = {
  id?: string;
  placeholder?: string;
  tags: Tag[];
  onTagsChange: (newTags: Tag[]) => void;
  disabled?: boolean;
  className?: string;
  containerClassName?: string;
};

export function InputWithInnerTags({
  id: providedId,
  placeholder = "Add a tag",
  tags,
  onTagsChange,
  disabled = false,
  className,
  containerClassName,
}: InputWithInnerTagsProps) {
  const generatedId = useId();
  const id = providedId || generatedId;
  const [inputValue, setInputValue] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const handleAddTag = () => {
    const trimmedValue = inputValue.trim();
    if (trimmedValue && !tags.some((tag) => tag.text === trimmedValue)) {
      const newTag: Tag = {
        id: nanoid(),
        text: trimmedValue,
      };
      onTagsChange([...tags, newTag]);
      setInputValue("");
    }
  };

  const handleRemoveTag = (tagId: string) => {
    onTagsChange(tags.filter((tag) => tag.id !== tagId));
  };

  useHotkeys(
    "enter",
    (e) => {
      e.preventDefault();
      if (inputRef.current === document.activeElement) {
        handleAddTag();
      }
    },
    {
      enableOnFormTags: true,
      preventDefault: true,
    },
    [inputValue, tags]
  );

  useHotkeys(
    "backspace",
    (e) => {
      if (
        inputRef.current === document.activeElement &&
        inputValue === "" &&
        tags.length > 0
      ) {
        e.preventDefault();
        const lastTag = tags.at(-1);
        if (lastTag) {
          handleRemoveTag(lastTag.id);
        }
      }
    },
    {
      enableOnFormTags: true,
    },
    [inputValue, tags]
  );

  return (
    <div className={containerClassName}>
      <div className="flex h-auto min-h-9 w-full min-w-0 flex-wrap items-center gap-2 rounded-md border border-input bg-input/20 px-3 py-2 text-sm shadow-xs outline-none transition-[color,box-shadow] focus-within:border-ring focus-within:ring-[3px] focus-within:ring-ring/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:bg-input/30 dark:aria-invalid:ring-destructive/40">
        {/* Render existing tags */}
        {tags.map((tag) => (
          <Badge
            className="inline-flex h-6 min-w-6 items-center justify-center gap-1 rounded border border-transparent bg-secondary px-2 font-medium text-secondary-foreground text-xs transition-colors hover:bg-secondary/80"
            key={tag.id}
            variant="secondary"
          >
            {tag.text}
            <Button
              aria-label={`Remove ${tag.text}`}
              className="ml-0.5 inline-flex size-4 h-auto items-center justify-center rounded px-0.5 text-secondary-foreground/70 transition-colors hover:bg-secondary-foreground/20 hover:text-secondary-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              disabled={disabled}
              onClick={() => handleRemoveTag(tag.id)}
              size="icon"
              type="button"
              variant="ghost"
            >
              <HugeiconsIcon className="size-3" icon={Cancel01Icon} />
            </Button>
          </Badge>
        ))}

        {/* Input for new tags */}
        <div className="flex min-w-[80px] flex-1 items-center gap-2">
          <Input
            className={`h-auto flex-1 border-none bg-transparent px-0 py-0 text-sm shadow-none outline-none placeholder:text-muted-foreground/70 focus:border-transparent focus:ring-0 focus:ring-offset-0 focus-visible:border-transparent focus-visible:outline-none focus-visible:ring-0 focus-visible:ring-offset-0 disabled:cursor-not-allowed ${className || ""}`}
            disabled={disabled}
            id={id}
            onChange={(e) => setInputValue(e.target.value)}
            placeholder={placeholder}
            ref={inputRef}
            value={inputValue}
          />
          {inputValue.trim() && (
            <Button
              className="h-6 w-6 p-0 text-muted-foreground hover:text-foreground"
              disabled={disabled}
              onClick={handleAddTag}
              size="icon"
              type="button"
              variant="ghost"
            >
              <HugeiconsIcon className="size-3.5" icon={Add01Icon} />
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
