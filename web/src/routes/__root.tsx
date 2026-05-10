import { Outlet } from "react-router";
import { PaperGrainOverlay } from "@/shared/layout/PaperGrainOverlay";

export default function RootLayout() {
  return (
    <div className="min-h-screen bg-paper text-ink">
      <PaperGrainOverlay />
      <Outlet />
    </div>
  );
}
