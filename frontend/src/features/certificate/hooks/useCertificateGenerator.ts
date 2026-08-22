import { useCallback, useState } from 'react'
import { generateCertificate } from '../service'

export function useCertificateGenerator() {
  const [open, setOpen] = useState(false)
  const [html, setHtml] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<Error | null>(null)

  const generate = useCallback(async (templateId: string, consignmentId: string, data: Record<string, unknown>) => {
    setLoading(true)
    setError(null)
    try {
      const result = await generateCertificate(templateId, consignmentId, data)
      setHtml(result)
      setOpen(true)
    } catch (err) {
      setError(err instanceof Error ? err : new Error('Failed to generate certificate'))
    } finally {
      setLoading(false)
    }
  }, [])

  return { generate, open, setOpen, html, loading, error }
}
