import { Menu, Plus } from "lucide-react";
import { UserMenu } from "@/features/auth/components/UserMenu";

type Props = {
  onMenuClick: () => void;
};

export function Topbar({ onMenuClick }: Props) {
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
          onClick={() => {
            // wired up in Library plan
            console.warn("Add Link not implemented yet");
          }}
        >
          <Plus className="h-5 w-5" strokeWidth={1.5} />
        </button>

        <UserMenu />
      </div>
    </header>
  );
}
