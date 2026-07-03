import { useAuthStore } from "@/features/auth/store";
import { SettingRow } from "./SettingRow";

export function AccountSection() {
  const user = useAuthStore((s) => s.user);
  if (!user) return null;

  const initial = user.displayName.charAt(0).toUpperCase() || "·";
  const role = user.isAdmin ? "Administrator" : "Member";

  return (
    <section className="px-4 lg:px-8 py-8 border-b border-rule">
      <div className="mb-6">
        <h2 className="display-tight text-2xl text-ink mb-1">Account</h2>
        <p className="font-body italic text-sm text-muted-foreground">
          Who you are to this instance.
        </p>
      </div>
      <div className="bg-paper-2 border border-rule p-6 md:p-8 flex flex-col gap-6">
        <div className="flex items-center gap-5">
          <div className="w-16 h-16 bg-ink text-paper flex items-center justify-center font-mono text-xl">
            {initial}
          </div>
          <div className="display-tight text-2xl text-ink">{user.displayName}</div>
        </div>
        <div className="flex flex-col gap-4 border-t border-rule pt-6">
          <SettingRow label="Email" value={user.email} />
          <SettingRow label="Role" value={role} />
        </div>
      </div>
    </section>
  );
}
