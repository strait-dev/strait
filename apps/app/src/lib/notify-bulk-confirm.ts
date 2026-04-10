export const bulkTopicConfirmTitle = (action: "add" | "remove"): string =>
  action === "add"
    ? "Bulk add subscribers to topic"
    : "Bulk remove subscribers from topic";

export const bulkTopicConfirmDescription = (
  action: "add" | "remove",
  count: number,
  topicKey: string
): string => {
  const noun = count === 1 ? "subscriber" : "subscribers";
  if (action === "add") {
    return `Add ${count} ${noun} to topic "${topicKey}"?`;
  }
  return `Remove ${count} ${noun} from topic "${topicKey}"? This cannot be undone.`;
};

export const bulkPrefConfirmDescription = (
  count: number,
  scope: string
): string => {
  const noun = count === 1 ? "subscriber" : "subscribers";
  return `Apply preference updates to ${count} ${noun} for scope "${scope}"?`;
};
