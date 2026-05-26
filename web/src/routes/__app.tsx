import { Outlet } from "react-router";
import { AppShell } from "@/shared/layout/AppShell";
import { AddLinkDialog } from "@/features/library/components/AddLinkDialog";
import { NewTopicDialog } from "@/features/radar/components/NewTopicDialog";

export default function AppLayout() {
  return (
    <AppShell>
      <Outlet />
      <AddLinkDialog />
      <NewTopicDialog />
    </AppShell>
  );
}
