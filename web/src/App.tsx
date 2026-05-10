import { createBrowserRouter } from "react-router";
import { RouterProvider } from "react-router/dom";
import RootLayout from "./routes/__root";
import AppLayout from "./routes/__app";
import PublicLayout from "./routes/_public";
import IndexRoute from "./routes/index";
import LoginRoute from "./routes/login";
import RegisterRoute from "./routes/register";
import LibraryListRoute from "./routes/library._index";
import LibraryItemRoute from "./routes/library.$id";
import SettingsRoute from "./routes/settings";
import NotFoundRoute from "./routes/not-found";

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
        element: <AppLayout />,
        children: [
          { path: "library", element: <LibraryListRoute /> },
          { path: "library/:id", element: <LibraryItemRoute /> },
          { path: "settings", element: <SettingsRoute /> },
        ],
      },
      { path: "*", element: <NotFoundRoute /> },
    ],
  },
]);

export default function App() {
  return <RouterProvider router={router} />;
}
