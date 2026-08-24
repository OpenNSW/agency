import { http, API_BASE_URL } from '@/http'

export async function generateCertificate(
  taskId: string,
  data: Record<string, unknown>,
  signal?: AbortSignal,
): Promise<string> {
  const res = await http.request({
    url: `${API_BASE_URL}/api/v1/applications/${taskId}/certificate`,
    method: 'POST',
    data: { data },
    attachToken: true,
    signal,
  })
  return res.data as string
}
