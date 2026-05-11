import { NavLink } from "react-router";
import { cn } from "@/shared/lib/cn";

const navItems = [
  { to: "/library", label: "Library", number: "01" },
  { to: "/radar", label: "Radar", number: "02", disabled: true },
  { to: "/settings", label: "Settings", number: "03" },
];

export function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  return (
    <aside className="flex h-full w-[280px] flex-col bg-paper-2 border-r border-rule">
      <div className="px-6 py-8 border-b border-rule">
        <p className="font-display italic text-3xl text-ink leading-none">Linktheca</p>
        <p className="label-sc mt-2 text-muted-foreground">A private archive</p>
      </div>

      <nav className="flex-1 px-6 py-6">
        <ul className="flex flex-col gap-3">
          {navItems.map((item) => (
            <li key={item.to}>
              {item.disabled ? (
                <span
                  className={cn(
                    "nav-item flex items-baseline gap-3 px-4 py-2 cursor-not-allowed opacity-50",
                  )}
                >
                  <span className="font-mono text-xs text-muted-foreground">{item.number}</span>
                  <span className="nav-label font-display text-lg">{item.label}</span>
                  <span className="label-sc text-muted-foreground ml-auto">soon</span>
                </span>
              ) : (
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
                  <span className="nav-number font-mono text-xs text-muted-foreground">{item.number}</span>
                  <span className="nav-label font-display text-lg text-ink-3">{item.label}</span>
                </NavLink>
              )}
            </li>
          ))}
        </ul>
      </nav>

      <div className="px-6 py-4 border-t border-rule">
        <p className="label-sc text-muted-foreground">v0.1.0 · self-hosted</p>
      </div>
    </aside>
  );
}
