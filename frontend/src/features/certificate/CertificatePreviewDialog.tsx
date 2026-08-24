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
              // allow-modals lets window.print() open the print dialog.
              // allow-same-origin lets the parent's contentWindow.print() call
              // reach the frame at all — Safari throws a SecurityError on that
              // call without it, treating the sandboxed frame as fully opaque,
              // even though Chrome/Firefox don't require it. Neither flag grants
              // script execution: allow-scripts is deliberately withheld, so
              // nothing in the certificate HTML (script tags, inline handlers,
              // javascript: URLs) can run regardless of origin.
              sandbox="allow-modals allow-same-origin"
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
