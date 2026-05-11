import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Link } from "react-router";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { Label } from "@/shared/ui/label";
import { login } from "@/features/auth/api";
import { ApiError } from "@/shared/api/errors";

const schema = z.object({
  email: z.string().email("Please enter a valid email address"),
  password: z.string().min(1, "Password is required"),
});

type FormValues = z.infer<typeof schema>;

function mapError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 401) return "Invalid email or password";
    if (err.status >= 500) return "Service unavailable. Please try again.";
    return err.message || "Something went wrong";
  }
  return "Something went wrong";
}

type Props = {
  onSuccess: () => void;
};

export function LoginForm({ onSuccess }: Props) {
  const [topError, setTopError] = useState<string | null>(null);
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { email: "", password: "" },
  });

  const onSubmit = handleSubmit(async (values) => {
    setTopError(null);
    try {
      await login(values);
      onSuccess();
    } catch (err) {
      setTopError(mapError(err));
    }
  });

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
      {topError && (
        <div
          role="alert"
          className="border border-vermillion bg-paper-2 px-4 py-3 text-sm text-vermillion-dark"
        >
          {topError}
        </div>
      )}

      <div className="flex flex-col gap-2">
        <Label htmlFor="email" className="label-sc text-ink-3">
          Email
        </Label>
        <Input
          id="email"
          type="email"
          autoComplete="email"
          aria-invalid={errors.email ? "true" : "false"}
          aria-describedby={errors.email ? "email-error" : undefined}
          {...register("email")}
        />
        {errors.email && (
          <p id="email-error" className="text-sm text-vermillion-dark">
            {errors.email.message}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="password" className="label-sc text-ink-3">
          Password
        </Label>
        <Input
          id="password"
          type="password"
          autoComplete="current-password"
          aria-invalid={errors.password ? "true" : "false"}
          aria-describedby={errors.password ? "password-error" : undefined}
          {...register("password")}
        />
        {errors.password && (
          <p id="password-error" className="text-sm text-vermillion-dark">
            {errors.password.message}
          </p>
        )}
      </div>

      <Button type="submit" disabled={isSubmitting}>
        {isSubmitting ? "Signing in…" : "Sign in"}
      </Button>

      <p className="label-sc text-center text-muted">
        <Link to="/register" className="hover:text-ink-3">
          Create account →
        </Link>
      </p>
    </form>
  );
}
