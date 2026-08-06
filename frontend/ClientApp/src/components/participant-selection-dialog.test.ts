import { mount } from '@vue/test-utils'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import { declaredPartyRoles } from '@/utils/participant-selection'
import ParticipantSelectionDialog from './ParticipantSelectionDialog.vue'

const ROLE_SUPPLIER = 'https://w3id.org/facis/dcs/taxonomy/v1#role-supplier'
const ROLE_CLIENT = 'https://w3id.org/facis/dcs/taxonomy/v1#role-client'

/**
 * command/create.go binds the originating organization to a party role and
 * attaches the reading organizations, both strictly conditional on fields the
 * create dialog never sent — so neither branch could run for a UI-made
 * contract, and a second organization could not be granted read access.
 */

beforeAll(() => {
  // jsdom implements neither dialog method.
  HTMLDialogElement.prototype.showModal = vi.fn()
  HTMLDialogElement.prototype.close = vi.fn()
})

function clickable(wrapper: ReturnType<typeof mount>, label: string) {
  return wrapper.findAll('button').find((button) => button.text() === label)
}

async function openDialog(partyRoles: { value: string; label: string }[]) {
  // The dialog body is teleported to document.body; stubbing Teleport keeps it
  // inside the wrapper so the fields are findable.
  const wrapper = mount(ParticipantSelectionDialog, {
    props: { partyRoles, roleState: partyRoles.length === 2 ? 'ready' : 'empty' },
    global: { stubs: { teleport: true } },
  })
  await clickable(wrapper, 'Create')?.trigger('click')
  return wrapper
}

describe('contract creation participants', () => {
  it('deduplicates both directions of top-level ODRL party references', () => {
    const template = {
      'dcs:policies': {
        'odrl:permission': [
          {
            'odrl:assigner': { '@id': `did:web:example.com:template#party-${encodeURIComponent(ROLE_SUPPLIER)}` },
            'odrl:assignee': { '@id': `did:web:example.com:template#party-${encodeURIComponent(ROLE_CLIENT)}` },
          },
        ],
        'odrl:obligation': [
          {
            'odrl:assigner': { '@id': `did:web:example.com:template#party-${encodeURIComponent(ROLE_CLIENT)}` },
            'odrl:assignee': { '@id': `did:web:example.com:template#party-${encodeURIComponent(ROLE_SUPPLIER)}` },
          },
        ],
        'odrl:prohibition': [
          {
            'odrl:assigner': { '@id': `did:web:example.com:template#party-${encodeURIComponent(ROLE_SUPPLIER)}` },
            'odrl:assignee': { '@id': `did:web:example.com:template#party-${encodeURIComponent(ROLE_CLIENT)}` },
          },
          {
            'odrl:assigner': { '@id': `did:web:example.com:template#party-${encodeURIComponent(ROLE_SUPPLIER)}` },
            'odrl:assignee': { '@id': `did:web:example.com:template#party-${encodeURIComponent(ROLE_CLIENT)}` },
          },
        ],
      },
    }

    expect(declaredPartyRoles(template)).toEqual([ROLE_SUPPLIER, ROLE_CLIENT])
    expect(declaredPartyRoles(undefined)).toEqual([])
  })

  it('submits the originator role and the reading organizations', async () => {
    const wrapper = await openDialog([
      { value: ROLE_SUPPLIER, label: 'supplier' },
      { value: ROLE_CLIENT, label: 'client' },
    ])

    expect(wrapper.find('select').element.value).toBe('')
    await wrapper.find('input[placeholder="did:web:..."]').setValue('did:web:peer.example')
    // The option shows the catalogue's own label, verbatim.
    expect(wrapper.findAll('select option').map((option) => option.text())).toEqual([
      'Select your role',
      'supplier',
      'client',
    ])
    await wrapper.find('select').setValue(ROLE_CLIENT)
    await wrapper.find('input[placeholder="Acme GmbH, Beispiel AG"]').setValue('Acme GmbH, Beispiel AG')
    await clickable(wrapper, 'Apply')?.trigger('click')

    expect(wrapper.emitted('submit')?.[0]).toEqual([
      {
        counterparty: 'did:web:peer.example',
        originatorRole: ROLE_CLIENT,
        parties: ['Acme GmbH', 'Beispiel AG'],
      },
    ])
  })

  it('never offers free text and explains that exactly two catalogued roles are required', async () => {
    const wrapper = await openDialog([])

    expect(wrapper.find('select').exists()).toBe(true)
    expect(wrapper.find('input[placeholder="e.g. provider"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('exactly two')
    expect(clickable(wrapper, 'Apply')?.attributes('disabled')).toBeDefined()
  })
})
