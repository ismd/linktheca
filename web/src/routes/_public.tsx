import { Outlet } from "react-router";
import { PublicMasthead } from "@/features/auth/components/PublicMasthead";

export default function PublicLayout() {
  return (
    <div className="paper-surface min-h-screen flex items-center justify-center p-8">
      <div className="w-full max-w-md">
        <PublicMasthead />
        <Outlet />
      </div>
    </div>
  );
}
