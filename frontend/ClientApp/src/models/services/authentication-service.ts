export interface AuthenticationService {
  loginPath: () => Promise<string>
  refresh: () => Promise<boolean>
  logout: () => void
}
