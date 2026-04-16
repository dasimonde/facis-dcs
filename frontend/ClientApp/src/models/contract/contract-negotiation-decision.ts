import type { NegotiationDecision } from "@/types/negotiation-decision";

export interface ContractNegotiationDecision {
  /** Negotiator who has to decide this negotiation decision */
  negotiator: string;
  decision?: NegotiationDecision;
  rejection_reason?: string;
}
