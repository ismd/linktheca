const KEY = "linktheca.refresh";

export function readRefreshToken(): string | null {
  try {
    return localStorage.getItem(KEY);
  } catch {
    return null;
  }
}

export function writeRefreshToken(token: string): void {
  try {
    localStorage.setItem(KEY, token);
  } catch {
    // ignore — storage may be disabled in private mode
  }
}

export function clearRefreshToken(): void {
  try {
    localStorage.removeItem(KEY);
  } catch {
    // ignore
  }
}
