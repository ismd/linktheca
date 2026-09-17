import { NavLink } from "react-router";
import { cn } from "@/shared/lib/cn";
import { APP_VERSION } from "@/shared/version";
import { useAuthStore } from "@/features/auth/store";

const baseNavItems = [
  { to: "/library", label: "Library" },
  { to: "/radar", label: "Radar" },
  { to: "/settings", label: "Settings" },
];

const adminNavItem = { to: "/admin/sources", label: "Admin" };

export function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  const isAdmin = useAuthStore((s) => s.user?.isAdmin ?? false);
  // Numbers follow position rather than being pinned to a label, so adding
  // Admin does not renumber the items above it.
  const navItems = isAdmin ? [...baseNavItems, adminNavItem] : baseNavItems;

  return (
    <aside className="flex h-full w-[280px] flex-col bg-paper-2 border-r border-rule">
      <div className="px-6 py-8 border-b border-rule">
        <p className="font-display italic text-3xl text-ink leading-none">Linktheca</p>
        <p className="label-sc mt-2 text-muted-foreground">A private archive</p>
      </div>

      <nav className="flex-1 px-6 py-6">
        <ul className="flex flex-col gap-3">
          {navItems.map((item, i) => (
            <li key={item.to}>
              <NavLink
                to={item.to}
                onClick={onNavigate}
                className={({ isActive }) =>
                  cn(
                    "nav-item flex items-baseline gap-3 px-4 py-2 hover:text-ink",
                    isActive && "active",
                  )
                }
              >
                <span className="nav-number font-mono text-xs text-muted-foreground">
                  {String(i + 1).padStart(2, "0")}
                </span>
                <span className="nav-label font-display text-xl text-ink-3">{item.label}</span>
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>

      <div className="px-6 py-4 border-t border-rule">
        <p className="label-sc text-muted-foreground">v{APP_VERSION} · self-hosted</p>
      </div>
    </aside>
  );
}
