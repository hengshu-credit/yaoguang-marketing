import { describe, expect, it } from 'vitest'
import { EMAIL_AI_SYSTEM_PROMPT, emailPurposeInstructions } from './email-ai-system-prompt'
import { BLOG_AI_SYSTEM_PROMPT } from '../blog/blog-ai-system-prompt'

describe('Yaoguang editor purpose and completion rules', () => {
  it('uses configured purpose for custom categories', () => {
    expect(emailPurposeInstructions('repayment_reminder', 'transactional')).toContain('Do not add')
    expect(emailPurposeInstructions('member_offer', 'marketing')).toContain('Include and preserve visible')
    expect(emailPurposeInstructions('marketing', 'transactional')).toContain('Transactional purpose')
  })
  it('keeps unknown categories conservative and respects built-in marketing', () => {
    expect(emailPurposeInstructions('opt_in', 'transactional')).toContain('{{ confirm_subscription_url }}')
    expect(emailPurposeInstructions('not-loaded')).toContain('purpose not resolved')
    expect(emailPurposeInstructions('marketing')).toContain('{{ unsubscribe_url }}')
  })
  it('requires body work before metadata, without introducing an editing loop', () => {
    expect(EMAIL_AI_SYSTEM_PROMPT).toContain('before updating the subject')
    expect(BLOG_AI_SYSTEM_PROMPT).toContain('before any title or SEO changes')
  })
})
