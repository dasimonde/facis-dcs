import http from '@/api/http'

export interface ContractSyncRequestResponse {
  from_peer_did: string
}

export async function requestContractSync(did: string): Promise<ContractSyncRequestResponse> {
  return http.get<ContractSyncRequestResponse>('/peer/contracts/sync', { params: { did } }).then((res) => res.data)
}
