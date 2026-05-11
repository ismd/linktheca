import { z } from "zod";
import type { User } from "./store";

export const RawUserSchema = z.object({
  id: z.number().int(),
  email: z.string(),
  display_name: z.string(),
  is_admin: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
});

export const TokenPairSchema = z.object({
  access_token: z.string().min(1),
  refresh_token: z.string().min(1),
});

export const AuthResponseSchema = z.object({
  user: RawUserSchema,
  tokens: TokenPairSchema,
});

export type RawUser = z.infer<typeof RawUserSchema>;
export type TokenPair = z.infer<typeof TokenPairSchema>;
export type AuthResponse = z.infer<typeof AuthResponseSchema>;

export function mapUser(raw: RawUser): User {
  return {
    id: raw.id,
    email: raw.email,
    displayName: raw.display_name,
    isAdmin: raw.is_admin,
  };
}
