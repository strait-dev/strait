"use client";

import dynamic from "next/dynamic";

const Toaster = dynamic(() => import("sonner").then((m) => m.Toaster), {
  ssr: false,
});

const LazyToaster = () => <Toaster />;

export default LazyToaster;
