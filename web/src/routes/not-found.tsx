import { Link } from "react-router";

export default function NotFoundRoute() {
  return (
    <div className="paper-surface min-h-screen flex flex-col items-center justify-center">
      <p className="font-display text-vermillion text-9xl leading-none">404</p>
      <p className="label-sc mt-4">Page not found</p>
      <Link to="/" className="mt-6 underline decoration-dotted text-ink">
        Back to library
      </Link>
    </div>
  );
}
