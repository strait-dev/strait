"use client";

import { createContext, useContext } from "react";

import { useMediaQuery } from "../hooks/use-media-query";
import { cn } from "../utils/index";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "./dialog";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "./drawer";

type BaseProps = {
  children: React.ReactNode;
};

type RootCredenzaProps = BaseProps & {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
};

type CredenzaProps = BaseProps & {
  className?: string;
  render?: React.ReactElement;
};

const CredenzaContext = createContext<{ isDesktop: boolean }>({
  isDesktop: false,
});

const useCredenzaContext = () => {
  const context = useContext(CredenzaContext);
  if (!context) {
    throw new Error(
      "Credenza components cannot be rendered outside the Credenza Context"
    );
  }
  return context;
};

const Credenza = ({ children, ...props }: RootCredenzaProps) => {
  const isDesktop = useMediaQuery("(min-width: 768px)");
  const Component = isDesktop ? Dialog : Drawer;

  return (
    <CredenzaContext.Provider value={{ isDesktop }}>
      <Component {...props} {...(!isDesktop && { autoFocus: true })}>
        {children}
      </Component>
    </CredenzaContext.Provider>
  );
};

const CredenzaTrigger = ({
  className,
  children,
  render,
  ...props
}: CredenzaProps) => {
  const { isDesktop } = useCredenzaContext();
  const Component = isDesktop ? DialogTrigger : DrawerTrigger;

  return (
    <Component className={className} render={render} {...props}>
      {children}
    </Component>
  );
};

const CredenzaClose = ({
  className,
  children,
  render,
  ...props
}: CredenzaProps) => {
  const { isDesktop } = useCredenzaContext();
  const Component = isDesktop ? DialogClose : DrawerClose;

  return (
    <Component className={className} render={render} {...props}>
      {children}
    </Component>
  );
};

const CredenzaContent = ({ className, children, ...props }: CredenzaProps) => {
  const { isDesktop } = useCredenzaContext();
  const Component = isDesktop ? DialogContent : DrawerContent;

  return (
    <Component className={className} {...props}>
      {children}
    </Component>
  );
};

const CredenzaDescription = ({
  className,
  children,
  ...props
}: CredenzaProps) => {
  const { isDesktop } = useCredenzaContext();
  const Component = isDesktop ? DialogDescription : DrawerDescription;

  return (
    <Component className={className} {...props}>
      {children}
    </Component>
  );
};

const CredenzaHeader = ({ className, children, ...props }: CredenzaProps) => {
  const { isDesktop } = useCredenzaContext();
  const Component = isDesktop ? DialogHeader : DrawerHeader;

  return (
    <Component className={className} {...props}>
      {children}
    </Component>
  );
};

const CredenzaTitle = ({ className, children, ...props }: CredenzaProps) => {
  const { isDesktop } = useCredenzaContext();
  const Component = isDesktop ? DialogTitle : DrawerTitle;

  return (
    <Component className={className} {...props}>
      {children}
    </Component>
  );
};

const CredenzaBody = ({ className, children, ...props }: CredenzaProps) => (
  <div className={cn("px-4 md:px-0", className)} {...props}>
    {children}
  </div>
);

const CredenzaFooter = ({ className, children, ...props }: CredenzaProps) => {
  const { isDesktop } = useCredenzaContext();
  const Component = isDesktop ? DialogFooter : DrawerFooter;

  return (
    <Component className={className} {...props}>
      {children}
    </Component>
  );
};

export {
  Credenza,
  CredenzaBody,
  CredenzaClose,
  CredenzaContent,
  CredenzaDescription,
  CredenzaFooter,
  CredenzaHeader,
  CredenzaTitle,
  CredenzaTrigger,
};
