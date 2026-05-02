type NotifyCursorItem = {
  created_at: string;
};

export const notifyCursorPageLimit = 25;

export const resolveNotifyNextCursor = <T extends NotifyCursorItem>(
  items: T[],
  limit = notifyCursorPageLimit
) => {
  if (items.length < limit) {
    return;
  }

  return items.at(-1)?.created_at;
};
