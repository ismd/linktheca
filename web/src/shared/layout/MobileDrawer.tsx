import * as Dialog from "@radix-ui/react-dialog";
import { Sidebar } from "@/shared/layout/Sidebar";
import { cn } from "@/shared/lib/cn";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function MobileDrawer({ open, onOpenChange }: Props) {
  const close = () => onOpenChange(false);
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay
          className={cn(
            "modal-backdrop fixed inset-0 z-30 lg:hidden",
            "data-[state=open]:animate-fade-in",
          )}
        />
        <Dialog.Content
          className={cn(
            "fixed inset-y-0 left-0 z-40 w-[280px] focus:outline-none lg:hidden",
            "data-[state=open]:animate-fade-in",
          )}
          aria-describedby={undefined}
        >
          <Dialog.Title className="sr-only">Navigation</Dialog.Title>
          <Sidebar onNavigate={close} />
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
