import { Navigate, Outlet, useLocation } from "react-router";
import { useAuthStore } from "@/features/auth/store";
import { FullPageSpinner } from "./FullPageSpinner";

// Cosmetic gate. The backend's RequireAdmin is what actually protects the data;
// this only keeps the screen out of a non-admin's way.
export function AdminRoute() {
  const status = useAuthStore((s) => s.status);
  const isAdmin = useAuthStore((s) => s.user?.isAdmin ?? false);
  const location = useLocation();

  if (status === "bootstrapping") return <FullPageSpinner />;
  if (status === "anonymous") {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }
  if (!isAdmin) return <Navigate to="/library" replace />;

  return <Outlet />;
}
