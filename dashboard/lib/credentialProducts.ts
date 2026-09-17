import type { BrokerCredential } from '@/lib/types'

export const INDIAN_PRODUCT_OPTIONS = [
  { value: 'MIS', label: 'MIS — Intraday', disabled: false },
  { value: 'CNC', label: 'CNC — Delivery', disabled: false },
  // F&O trading isn't supported yet — keep NRML visible but unselectable.
  { value: 'NRML', label: 'NRML — Overnight F&O, coming soon', disabled: true },
] as const

export type IndianProduct = (typeof INDIAN_PRODUCT_OPTIONS)[number]['value']

export function parseCredentialProducts(cred: Pick<BrokerCredential, 'product' | 'products'>): IndianProduct[] {
  if (cred.products?.length) {
    return cred.products.filter(isIndianProduct)
  }
  const raw = cred.product?.trim()
  if (!raw) return ['MIS']
  if (raw.includes(',')) {
    return raw.split(',').map(p => p.trim().toUpperCase()).filter(isIndianProduct)
  }
  return isIndianProduct(raw) ? [raw as IndianProduct] : ['MIS']
}

function isIndianProduct(value: string): value is IndianProduct {
  return value === 'MIS' || value === 'CNC' || value === 'NRML'
}

export function formatCredentialProducts(cred: Pick<BrokerCredential, 'product' | 'products'>): string {
  return parseCredentialProducts(cred).join(', ')
}

export function buildCredentialProductPayload(
  products: IndianProduct[],
  multiAllowed: boolean,
): { product?: string; products?: string[] } {
  if (multiAllowed) {
    return { products }
  }
  return { product: products[0] ?? 'MIS' }
}
