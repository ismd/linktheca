import { Navigate, Outlet, useLocation } from "react-router";
import { useAuthStore } from "@/features/auth/store";
import { FullPageSpinner } from "./FullPageSpinner";

export function ProtectedRoute() {
  const status = useAuthStore((s) => s.status);
  const location = useLocation();

  if (status === "bootstrapping") return <FullPageSpinner />;
  if (status === "anonymous") {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }

  return <Outlet />;
}
