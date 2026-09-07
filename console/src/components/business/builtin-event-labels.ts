import { msg } from '@lingui/core/macro'
import type { I18n, MessageDescriptor } from '@lingui/core'
import type { BusinessDefinition } from '../../services/api/business'

const labels: Record<string, MessageDescriptor> = {
 'sms.sent': msg`SMS sent`,
 'sms.delivered': msg`SMS delivered`,
 'sms.opened': msg`SMS opened`,
 'sms.clicked': msg`SMS clicked`,
 'sms.failed': msg`SMS failed`,
 'sms.bounced': msg`SMS bounced`,
 'sms.complained': msg`SMS complained`,
 'sms.unsubscribed': msg`SMS unsubscribed`,
 'sms.updated': msg`SMS updated`,
 'push.sent': msg`Push sent`,
 'push.delivered': msg`Push delivered`,
 'push.opened': msg`Push opened`,
 'push.clicked': msg`Push clicked`,
 'push.failed': msg`Push failed`,
 'push.bounced': msg`Push bounced`,
 'push.complained': msg`Push complained`,
 'push.unsubscribed': msg`Push unsubscribed`,
 'push.updated': msg`Push updated`,
 'whatsapp.sent': msg`WhatsApp sent`,
 'whatsapp.delivered': msg`WhatsApp delivered`,
 'whatsapp.opened': msg`WhatsApp opened`,
 'whatsapp.clicked': msg`WhatsApp clicked`,
 'whatsapp.failed': msg`WhatsApp failed`,
 'whatsapp.bounced': msg`WhatsApp bounced`,
 'whatsapp.complained': msg`WhatsApp complained`,
 'whatsapp.unsubscribed': msg`WhatsApp unsubscribed`,
 'whatsapp.updated': msg`WhatsApp updated`,
 'webhook.sent': msg`Webhook sent`,
 'webhook.delivered': msg`Webhook delivered`,
 'webhook.opened': msg`Webhook opened`,
 'webhook.clicked': msg`Webhook clicked`,
 'webhook.failed': msg`Webhook failed`,
 'webhook.bounced': msg`Webhook bounced`,
 'webhook.complained': msg`Webhook complained`,
 'webhook.unsubscribed': msg`Webhook unsubscribed`,
 'webhook.updated': msg`Webhook updated`,
 'contact.created': msg`Customer created`, 'contact.updated': msg`Customer updated`, 'email.sent': msg`Email sent`, 'email.delivered': msg`Email delivered`, 'email.opened': msg`Email opened`, 'email.clicked': msg`Email clicked`, 'email.bounced': msg`Email bounced`, 'email.complained': msg`Email complained`, 'email.unsubscribed': msg`Email unsubscribed`, 'state.entered': msg`State entered`, 'state.exited': msg`State exited`
}
export function businessDefinitionDisplayName(definition: BusinessDefinition, i18n: Pick<I18n, '_'>): string {
 const message = definition.kind === 'event' && definition.settings.builtin === true ? labels[definition.key] : undefined
 return message ? i18n._(message) : definition.name
}
