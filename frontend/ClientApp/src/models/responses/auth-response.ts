export interface AuthCallbackResponse {
  access_token: string
  expires_in: number
  token_type: string
}

export interface LoginResponse {
  auth_url: string
}

export interface LogoutResponse {
  logout_url: string
}
