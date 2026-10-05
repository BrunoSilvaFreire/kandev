"use client";

import { memo } from "react";
import type { DocumentCommentContextItem } from "@/lib/types/context";
import { SelectionCommentItem } from "./selection-comment-item";

export const DocumentCommentItem = memo(function DocumentCommentItem({
  item,
}: {
  item: DocumentCommentContextItem;
}) {
  return (
    <SelectionCommentItem
      kind="document-comment"
      label={item.label}
      comments={item.comments}
      onRemove={item.onRemove}
    />
  );
});
