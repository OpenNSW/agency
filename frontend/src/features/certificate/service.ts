import { http, API_BASE_URL } from '@/http'

export async function generateCertificate(
  templateId: string,
  consignmentId: string,
  data: Record<string, unknown>,
  signal?: AbortSignal,
): Promise<string> {
  const res = await http.request({
    url: `${API_BASE_URL}/api/v1/certificates/generate`,
    method: 'POST',
    data: { templateId, consignmentId, data },
    attachToken: true,
    signal,
  })
  return res.data as string
}
