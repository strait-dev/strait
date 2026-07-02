const reactElementType = Symbol.for("react.element");
const reactTransitionalElementType = Symbol.for("react.transitional.element");
const reactFragmentType = Symbol.for("react.fragment");

export function isFragment(value: unknown): boolean {
  if (typeof value !== "object" || value === null) {
    return false;
  }

  const element = value as { $$typeof?: symbol; type?: unknown };
  return (
    (element.$$typeof === reactElementType ||
      element.$$typeof === reactTransitionalElementType) &&
    element.type === reactFragmentType
  );
}
