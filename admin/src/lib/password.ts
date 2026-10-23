import { z } from 'zod'
import type { MessageKey } from '@/i18n'

type T = (k: MessageKey) => string

// The server's policy: 8 characters, a lowercase letter, an uppercase letter,
// a digit, 72 bytes at most.
export const passwordSchema = (t: T) =>
  z
    .string()
    .min(8, t('validation.password'))
    .regex(/[a-z]/, t('validation.password'))
    .regex(/[A-Z]/, t('validation.password'))
    .regex(/\d/, t('validation.password'))
    .refine((v) => new TextEncoder().encode(v).length <= 72, t('validation.passwordBytes'))

export const emailSchema = (t: T) => z.string().min(1, t('validation.required')).email(t('validation.email'))
