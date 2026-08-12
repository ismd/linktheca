import { createBrowserRouter } from "react-router";
import { RouterProvider } from "react-router/dom";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/shared/api/query-client";
import { useBootstrap } from "@/features/auth/use-bootstrap";
import { ProtectedRoute } from "@/shared/layout/ProtectedRoute";
import { Toaster } from "@/shared/ui/sonner";
import RootLayout from "./routes/__root";
import PublicLayout from "./routes/_public";
import AppLayout from "./routes/__app";
import IndexRoute from "./routes/index";
import LoginRoute from "./routes/login";
import RegisterRoute from "./routes/register";
import LibraryListRoute from "./routes/library._index";
import LibraryItemRoute from "./routes/library.$id";
import SettingsRoute from "./routes/settings";
import NotFoundRoute from "./routes/not-found";
import RadarInboxRoute from "./routes/radar._index";
import TopicsListRoute from "./routes/radar.topics._index";
import TopicRoute from "./routes/radar.topics.$topicId";
import MatchRoute from "./routes/radar.matches.$matchId";

const router = createBrowserRouter([
  {
    element: <RootLayout />,
    children: [
      { index: true, element: <IndexRoute /> },
      {
        element: <PublicLayout />,
        children: [
          { path: "login", element: <LoginRoute /> },
          { path: "register", element: <RegisterRoute /> },
        ],
      },
      {
        element: <ProtectedRoute />,
        children: [
          {
            element: <AppLayout />,
            children: [
              { path: "library", element: <LibraryListRoute /> },
              { path: "library/:id", element: <LibraryItemRoute /> },
              { path: "radar", element: <RadarInboxRoute /> },
              { path: "radar/topics", element: <TopicsListRoute /> },
              { path: "radar/topics/:topicId", element: <TopicRoute /> },
              { path: "radar/matches/:matchId", element: <MatchRoute /> },
              { path: "settings", element: <SettingsRoute /> },
            ],
          },
        ],
      },
      { path: "*", element: <NotFoundRoute /> },
    ],
  },
]);

function BootstrapGate({ children }: { children: React.ReactNode }) {
  useBootstrap();
  return <>{children}</>;
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BootstrapGate>
        <RouterProvider router={router} />
        <Toaster />
      </BootstrapGate>
    </QueryClientProvider>
  );
}
