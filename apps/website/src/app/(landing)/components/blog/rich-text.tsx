import type { RichTextProps } from "basehub/react-rich-text";

export const richTextComponents: RichTextProps["components"] = {
  h1: ({ children }) => (
    <h1 className="mb-4 text-4xl tracking-tighter sm:text-5xl lg:text-6xl">
      {children}
    </h1>
  ),
  h2: ({ children }) => (
    <h2 className="mb-4 text-3xl tracking-tighter sm:text-4xl">{children}</h2>
  ),
  h3: ({ children }) => (
    <h3 className="mb-4 text-2xl tracking-tighter sm:text-3xl">{children}</h3>
  ),
  h4: ({ children }) => (
    <h4 className="mb-4 text-xl tracking-tighter sm:text-2xl">{children}</h4>
  ),
  h5: ({ children }) => (
    <h5 className="mb-4 text-lg tracking-tighter sm:text-xl">{children}</h5>
  ),
  h6: ({ children }) => (
    <h6 className="mb-4 text-base tracking-tighter sm:text-base">{children}</h6>
  ),
  br: () => <br />,
  p: ({ children }) => (
    <p className="mb-4 text-base text-muted-foreground leading-relaxed sm:leading-8">
      {children}
    </p>
  ),
};
