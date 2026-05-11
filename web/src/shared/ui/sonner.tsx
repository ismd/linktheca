import { Toaster as SonnerToaster } from "sonner";

export function Toaster() {
  return (
    <SonnerToaster
      position="top-right"
      richColors={false}
      toastOptions={{
        className:
          "border border-rule bg-paper-2 text-ink font-body text-sm shadow-md",
      }}
    />
  );
}
