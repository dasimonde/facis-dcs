import type { ContractNegotiationDecision } from "./contract-negotiation-decision";

export interface ContractNegotiation {
  id: string;
  change_request: unknown;
  created_by: string;
  created_at: string;
  negotiation_decisions: ContractNegotiationDecision[];
}
