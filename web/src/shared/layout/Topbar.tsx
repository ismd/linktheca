import { Menu, Plus } from "lucide-react";
import { UserMenu } from "@/features/auth/components/UserMenu";
import { useAddLinkStore } from "@/features/library/use-add-link-store";

type Props = {
  onMenuClick: () => void;
};

export function Topbar({ onMenuClick }: Props) {
  const openAddLink = useAddLinkStore((s) => s.open);

  return (
    <header className="sticky top-0 z-10 h-16 bg-paper-2 border-b border-rule flex items-center px-4 lg:px-6">
      <button
        type="button"
        onClick={onMenuClick}
        aria-label="Open navigation"
        className="icon-btn lg:hidden"
      >
        <Menu className="h-5 w-5" strokeWidth={1.5} />
      </button>

      <div className="ml-auto flex items-center gap-3">
        <button
          type="button"
          aria-label="Add link"
          className="icon-btn"
          onClick={openAddLink}
        >
          <Plus className="h-5 w-5" strokeWidth={1.5} />
        </button>

        <UserMenu />
      </div>
    </header>
  );
}
