import { useAuthStore } from "@/features/auth/store";
import { useLogout } from "@/features/auth/use-logout";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@/shared/ui/dropdown-menu";

export function UserMenu() {
  const user = useAuthStore((s) => s.user);
  const logout = useLogout();

  if (!user) return null;

  const initial = user.displayName.charAt(0).toUpperCase() || "·";

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        aria-label="Open user menu"
        className="btn btn-outline btn-icon-md"
      >
        <span className="font-display text-lg normal-case tracking-normal">{initial}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel>
          <span className="block text-ink-2 normal-case text-sm font-body tracking-normal">
            {user.displayName}
          </span>
          <span className="block text-xs text-muted-foreground normal-case tracking-normal mt-0.5">
            {user.email}
          </span>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => void logout()}>Sign out</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
