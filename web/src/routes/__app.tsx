import { Outlet } from "react-router";
import { AppShell } from "@/shared/layout/AppShell";

export default function AppLayout() {
  return (
    <AppShell>
      <Outlet />
    </AppShell>
  );
}
