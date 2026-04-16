export interface UserAllRequest {
  offset?: number
  limit?: number
}

export interface UserRolesByUserIdRequest {
  userId: string
}
