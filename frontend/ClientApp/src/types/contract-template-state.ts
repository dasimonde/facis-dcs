export type ContractTemplateState = (typeof TemplateState)[keyof typeof TemplateState]

export const TemplateState = {
    draft: 'DRAFT',
    submitted: 'SUBMITTED',
    rejected: 'REJECTED',
    reviewed: 'REVIEWED',
    approved: 'APPROVED',
    deleted: 'DELETED',
    deprecated: 'DEPRECATED',
    registered: 'REGISTERED',
} as const

export const contractTemplateStates: ContractTemplateState[] = Object.values(TemplateState)
