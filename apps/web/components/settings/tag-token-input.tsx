"use client";

import { useCallback, useId, useState } from "react";
import { IconX } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import {
  MAX_PROFILE_TAG_SIZE,
  canonicalizeTags,
} from "@/lib/agent-profile-tags";

export function areTagListsEqual(a?: string[], b?: string[]): boolean {
  const left = a ?? [];
  const right = b ?? [];
  if (left.length !== right.length) return false;
  return left.every((tag, i) => tag === right[i]);
}

type TagTokenInputProps = {
  tags?: string[];
  onChange: (tags: string[]) => void;
  placeholder: string;
  removeLabel: (tag: string) => string;
  disabled?: boolean;
  id?: string;
  listTestId?: string;
  inputTestId?: string;
};

/**
 * Comma/Enter token input shared by profile tags and workflow allowed tags.
 * Canonicalizes on commit and never emits an empty or oversized token.
 */
export function TagTokenInput({
  tags,
  onChange,
  placeholder,
  removeLabel,
  disabled = false,
  id,
  listTestId,
  inputTestId,
}: TagTokenInputProps) {
  const generatedId = useId();
  const inputId = id ?? generatedId;
  const [text, setText] = useState("");
  const current = tags ?? [];

  const commit = useCallback(
    (next: string[]) => {
      const canonical = canonicalizeTags(next);
      if (!areTagListsEqual(canonical, current)) onChange(canonical);
    },
    [current, onChange],
  );

  const addTags = useCallback(
    (raw: string) => {
      const incoming = raw
        .split(",")
        .map((part) => part.trim().toLowerCase())
        .filter((part) => part.length > 0 && part.length <= MAX_PROFILE_TAG_SIZE);
      if (incoming.length === 0) return;
      commit([...current, ...incoming]);
    },
    [current, commit],
  );

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLInputElement>) => {
      if (disabled) return;
      if (event.key === "Enter" || event.key === ",") {
        event.preventDefault();
        addTags(text);
        setText("");
        return;
      }
      if (event.key === "Backspace" && text === "" && current.length > 0) {
        event.preventDefault();
        commit(current.slice(0, -1));
      }
    },
    [disabled, text, current, addTags, commit],
  );

  return (
    <div className="space-y-2">
      {current.length > 0 && (
        <div className="flex flex-wrap gap-2" data-testid={listTestId}>
          {current.map((tag) => (
            <Badge key={tag} variant="secondary" className="gap-1 pr-1">
              <span>{tag}</span>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-6 w-6 cursor-pointer max-md:h-11 max-md:w-11 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
                disabled={disabled}
                aria-label={removeLabel(tag)}
                onClick={() => commit(current.filter((entry) => entry !== tag))}
              >
                <IconX className="h-3 w-3" />
              </Button>
            </Badge>
          ))}
        </div>
      )}
      <Input
        id={inputId}
        value={text}
        placeholder={placeholder}
        disabled={disabled}
        onChange={(event) => setText(event.target.value)}
        onKeyDown={handleKeyDown}
        onBlur={() => {
          if (text.trim()) {
            addTags(text);
            setText("");
          }
        }}
        data-testid={inputTestId}
        className="h-7 max-md:h-11 [@media(pointer:coarse)]:h-11"
      />
    </div>
  );
}
