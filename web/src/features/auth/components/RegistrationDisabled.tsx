import { Link } from "react-router";

export function RegistrationDisabled() {
  return (
    <div className="text-center">
      <p className="font-display text-2xl text-ink-2 mb-3">Registration is closed</p>
      <p className="text-sm text-ink-3 mb-6">
        New accounts are disabled on this instance.
      </p>
      <p className="label-sc text-muted">
        <Link to="/login" className="hover:text-ink-3">
          ← Back to sign in
        </Link>
      </p>
    </div>
  );
}
