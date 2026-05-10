import { Menu, Plus } from "lucide-react";

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

        <div
          className="h-9 w-9 rounded-none border border-rule bg-paper flex items-center justify-center"
          aria-label="User menu placeholder"
        >
          <span className="font-display text-lg text-ink">L</span>
        </div>
      </div>
    </header>
  );
}
