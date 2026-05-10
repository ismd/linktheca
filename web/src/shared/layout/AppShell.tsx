import { useState } from "react";
import { Sidebar } from "@/shared/layout/Sidebar";
import { Topbar } from "@/shared/layout/Topbar";
import { MobileDrawer } from "@/shared/layout/MobileDrawer";

type Props = {
  children: React.ReactNode;
};

export function AppShell({ children }: Props) {
  const [drawerOpen, setDrawerOpen] = useState(false);

  return (
    <div className="min-h-screen">
      <div className="hidden lg:block fixed inset-y-0 left-0 z-20">
        <Sidebar />
      </div>

      <MobileDrawer open={drawerOpen} onOpenChange={setDrawerOpen} />

      <div className="lg:pl-[280px] flex flex-col min-h-screen">
        <Topbar onMenuClick={() => setDrawerOpen(true)} />
        <main className="flex-1">{children}</main>
      </div>
    </div>
  );
}
