import { Outlet } from "react-router";
import { AppShell } from "@/shared/layout/AppShell";
import { AddLinkDialog } from "@/features/library/components/AddLinkDialog";

export default function AppLayout() {
  return (
    <AppShell>
      <Outlet />
      <AddLinkDialog />
    </AppShell>
  );
}
