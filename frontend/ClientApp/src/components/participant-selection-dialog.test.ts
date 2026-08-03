import { mount } from '@vue/test-utils'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import { declaredPartyRoles } from '@/utils/participant-selection'
import ParticipantSelectionDialog from './ParticipantSelectionDialog.vue'

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

async function openDialog(partyRoles: string[]) {
  // The dialog body is teleported to document.body; stubbing Teleport keeps it
  // inside the wrapper so the fields are findable.
  const wrapper = mount(ParticipantSelectionDialog, {
    props: { partyRoles },
    global: { stubs: { teleport: true } },
  })
  await clickable(wrapper, 'Create')?.trigger('click')
  return wrapper
}

describe('contract creation participants', () => {
  it('reads the roles a template declares off its party placeholders', () => {
    const template = {
      'dcs:parties': [
        { '@id': 'did:web:example.com:template#party-provider' },
        { '@id': 'did:web:example.com:template#party-customer' },
        { '@id': 'did:web:example.com:template#not-a-party' },
      ],
    }

    expect(declaredPartyRoles(template)).toEqual(['provider', 'customer'])
    expect(declaredPartyRoles(undefined)).toEqual([])
  })

  it('submits the originator role and the reading organizations', async () => {
    const wrapper = await openDialog(['provider', 'customer'])

    await wrapper.find('input[placeholder="did:web:..."]').setValue('did:web:peer.example')
    await wrapper.find('select').setValue('customer')
    await wrapper.find('input[placeholder="Acme GmbH, Beispiel AG"]').setValue('Acme GmbH, Beispiel AG')
    await clickable(wrapper, 'Apply')?.trigger('click')

    expect(wrapper.emitted('submit')?.[0]).toEqual([
      {
        counterparty: 'did:web:peer.example',
        originatorRole: 'customer',
        parties: ['Acme GmbH', 'Beispiel AG'],
      },
    ])
  })

  it('falls back to a free-text role when the template declares none', async () => {
    const wrapper = await openDialog([])

    expect(wrapper.find('select').exists()).toBe(false)
    await wrapper.find('input[placeholder="e.g. provider"]').setValue('provider')
    await clickable(wrapper, 'Apply')?.trigger('click')

    expect(wrapper.emitted('submit')?.[0]).toEqual([{ counterparty: '', originatorRole: 'provider', parties: [] }])
  })
})
