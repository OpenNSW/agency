import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Dialog, Flex, Box } from '@radix-ui/themes'

interface CertificatePreviewDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  html: string | null
}

export function CertificatePreviewDialog({ open, onOpenChange, html }: CertificatePreviewDialogProps) {
  const { t } = useTranslation()
  const frameRef = useRef<HTMLIFrameElement>(null)

  const handlePrint = () => {
    frameRef.current?.contentWindow?.print()
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Content maxWidth="900px">
        <Dialog.Title>{t('consignments.detail.certificate.title')}</Dialog.Title>
        {html && (
          <Box mt="3" mb="4" style={{ border: '1px solid var(--gray-a5)', borderRadius: 'var(--radius-3)' }}>
            <iframe
              ref={frameRef}
              title={t('consignments.detail.certificate.title')}
              srcDoc={html}
              style={{ width: '100%', height: '70vh', border: 'none', borderRadius: 'var(--radius-3)' }}
            />
          </Box>
        )}
        <Flex justify="end" gap="3">
          <Dialog.Close>
            <Button variant="soft" color="gray">
              {t('consignments.detail.certificate.close')}
            </Button>
          </Dialog.Close>
          <Button onClick={handlePrint}>{t('consignments.detail.certificate.print')}</Button>
        </Flex>
      </Dialog.Content>
    </Dialog.Root>
  )
}
