import { renderHook, waitFor } from "@testing-library/react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { useHydratedTableData } from "../use-hydrated-table-data";

type Row = {
  id: string;
};

function ServerProbe({
  data,
  onSnapshot,
}: {
  data: Row[];
  onSnapshot: (value: ReturnType<typeof useHydratedTableData<Row>>) => void;
}) {
  const value = useHydratedTableData(data);
  onSnapshot(value);
  return null;
}

describe("useHydratedTableData", () => {
  it("renders a stable empty loading shell during server render", () => {
    let snapshot: ReturnType<typeof useHydratedTableData<Row>> | null = null;

    renderToString(
      <ServerProbe
        data={[{ id: "row-1" }]}
        onSnapshot={(value) => {
          snapshot = value;
        }}
      />
    );

    expect(snapshot).toEqual({
      data: [],
      isHydrated: false,
      isLoading: true,
    });
  });

  it("switches to real rows after client hydration", async () => {
    const rows = [{ id: "row-1" }, { id: "row-2" }];
    const { result } = renderHook(() => useHydratedTableData(rows));

    await waitFor(() => {
      expect(result.current.isHydrated).toBe(true);
    });

    expect(result.current).toEqual({
      data: rows,
      isHydrated: true,
      isLoading: false,
    });
  });

  it("keeps true empty data empty after hydration", async () => {
    const { result } = renderHook(() => useHydratedTableData<Row>([]));

    await waitFor(() => {
      expect(result.current.isHydrated).toBe(true);
    });

    expect(result.current).toEqual({
      data: [],
      isHydrated: true,
      isLoading: false,
    });
  });
});
