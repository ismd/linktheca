import { Outlet } from "react-router";

export default function PublicLayout() {
  return (
    <div className="paper-surface min-h-screen flex items-center justify-center p-8">
      <div className="w-full max-w-md">
        <Outlet />
      </div>
    </div>
  );
}
