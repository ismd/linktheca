import { Menu, Plus } from "lucide-react";
import { Button } from "@/shared/ui/button";
import { UserMenu } from "@/features/auth/components/UserMenu";
import { useAddLinkStore } from "@/features/library/use-add-link-store";

type Props = {
  onMenuClick: () => void;
};

export function Topbar({ onMenuClick }: Props) {
  const openAddLink = useAddLinkStore((s) => s.open);

  return (
    <header className="sticky top-0 z-10 h-16 bg-paper-2 border-b border-rule flex items-center px-4 lg:px-6">
      <Button
        variant="outline"
        size="icon"
        onClick={onMenuClick}
        aria-label="Open navigation"
        className="lg:hidden"
      >
        <Menu className="h-5 w-5" strokeWidth={1.5} />
      </Button>

      <div className="ml-auto flex items-center gap-3">
        <Button
          variant="outline"
          size="icon"
          aria-label="Add link"
          onClick={openAddLink}
        >
          <Plus className="h-5 w-5" strokeWidth={1.5} />
        </Button>

        <UserMenu />
      </div>
    </header>
  );
}
