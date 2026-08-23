import { useState, useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Button, Badge, Spinner, Text, Card, Flex, Box, Callout } from '@radix-ui/themes'
import {
  ArrowLeftIcon,
  CheckCircledIcon,
  ExclamationTriangleIcon,
  InfoCircledIcon,
  ChatBubbleIcon,
  FileTextIcon,
} from '@radix-ui/react-icons'
import { type AgencyApplication } from './types'
import { JsonForms } from '@jsonforms/react'
import { radixRenderers } from '@opennsw/jsonforms-renderers'
import { createAjv, type JsonSchema, type UISchemaElement } from '@jsonforms/core'
import { fetchApplicationDetail, submitReview } from './service'
import { type SchemaProperty } from './types'
import { useCertificateGenerator } from '@/features/certificate/hooks/useCertificateGenerator'
import { CertificatePreviewDialog } from '@/features/certificate/CertificatePreviewDialog'

export function ApplicationDetailScreen() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()

  const [searchParams] = useSearchParams()
  const taskId = searchParams.get('taskId')

  const [application, setApplication] = useState<AgencyApplication | null>(null)
  const [loading, setLoading] = useState(true)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)

  const [agencyFormConfig, setAgencyFormConfig] = useState<{ schema: JsonSchema; uiSchema: UISchemaElement } | null>(
    null,
  )
  const [agencyFormData, setAgencyFormData] = useState<Record<string, unknown>>({})
  const [formErrors, setFormErrors] = useState<unknown[]>([])
  const [showErrors, setShowErrors] = useState(false)

  const ajvInstance = useMemo(() => createAjv({ useDefaults: true }), [])

  const certificate = useCertificateGenerator()

  const isCertificateDataReady = useMemo(() => {
    if (!application?.certificateDataSchema) return true
    return !!ajvInstance.validate(application.certificateDataSchema, agencyFormData)
  }, [ajvInstance, application, agencyFormData])

  const canGenerateCertificate = (application?.allowedActions?.includes('REVIEW') ?? false) && isCertificateDataReady

  const handleGenerateCertificate = () => {
    if (!application?.certificateTemplateId || !taskId || !canGenerateCertificate) return
    void certificate.generate(taskId, agencyFormData)
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!taskId || !application) {
      setError(t('errors.dataUnavailable'))
      return
    }

    let hasErrors = formErrors.length > 0
    if (agencyFormConfig) {
      const isValid = ajvInstance.validate(agencyFormConfig.schema, agencyFormData)
      if (!isValid) {
        hasErrors = true
      }
    }

    if (hasErrors) {
      setError(t('errors.validationErrors'))
      setShowErrors(true)
      return
    }
    setIsSubmitting(true)
    setError(null)
    try {
      await submitReview(taskId, agencyFormData)
      setSuccess(true)
      setTimeout(() => navigate('/consignments'), 500)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('errors.submitFailed'))
    } finally {
      setIsSubmitting(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    async function fetchData() {
      if (!taskId) {
        setError(t('errors.noTaskId'))
        setLoading(false)
        return
      }
      try {
        const data = await fetchApplicationDetail(taskId, controller.signal)
        setApplication(data)
        if (data.agencyForm) {
          const schema = structuredClone(data.agencyForm.schema)
          const capitalizeOptions = (prop: SchemaProperty) => {
            if (prop.oneOf) {
              prop.oneOf = prop.oneOf.map((opt) => {
                const titleVal = opt.title || String(opt.const)
                const formattedTitle = titleVal
                  .split(/[_\s]+/)
                  .map((word) => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
                  .join(' ')
                return { ...opt, title: formattedTitle }
              })
            } else if (prop.enum) {
              prop.oneOf = prop.enum.map((val: string) => {
                const title = val
                  .split(/[_\s]+/)
                  .map((word) => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
                  .join(' ')
                return { const: val, title }
              })
              delete prop.enum
            }
          }
          if (schema.properties) {
            Object.values(schema.properties).forEach((prop) => {
              capitalizeOptions(prop as SchemaProperty)
            })
          }
          setAgencyFormConfig({ schema, uiSchema: data.agencyForm.uiSchema })
        } else {
          setAgencyFormConfig(null)
        }
        setAgencyFormData(data.agencyActionData || {})
        setShowErrors(false)
      } catch (err) {
        if (err instanceof Error && err.name === 'AbortError') return
        setError(t('errors.loadFailed'))
        console.error(err)
      } finally {
        if (!controller.signal.aborted) setLoading(false)
      }
    }
    void fetchData()
    return () => controller.abort()
  }, [taskId, t])

  if (loading) {
    return (
      <Flex align="center" justify="center" py="9">
        <Spinner size="3" />
        <Text size="3" color="gray" ml="3">
          {t('consignments.detail.loading')}
        </Text>
      </Flex>
    )
  }

  if (error && !application) {
    return (
      <Box p="6">
        <Callout.Root color="red">
          <Callout.Icon>
            <ExclamationTriangleIcon />
          </Callout.Icon>
          <Callout.Text>{error}</Callout.Text>
        </Callout.Root>
        <Button
          variant="soft"
          mt="4"
          onClick={() => {
            void navigate('/consignments')
          }}
        >
          <ArrowLeftIcon /> {t('consignments.detail.backToList')}
        </Button>
      </Box>
    )
  }

  if (!application) {
    return (
      <Box p="6">
        <Callout.Root color="red">
          <Callout.Icon>
            <ExclamationTriangleIcon />
          </Callout.Icon>
          <Callout.Text>{t('consignments.detail.notFound')}</Callout.Text>
        </Callout.Root>
        <Button
          variant="soft"
          mt="4"
          onClick={() => {
            void navigate('/consignments')
          }}
        >
          <ArrowLeftIcon /> {t('consignments.detail.backToList')}
        </Button>
      </Box>
    )
  }

  const canReview = application.allowedActions?.includes('REVIEW') ?? false
  const isActionable = application.status === 'PENDING' && canReview

  const statusColor =
    application.status === 'APPROVED'
      ? 'green'
      : application.status === 'REJECTED'
        ? 'red'
        : application.status === 'FEEDBACK_REQUESTED'
          ? 'amber'
          : 'blue'

  return (
    <div className="animate-fade-in max-w-6xl mx-auto">
      <Flex justify="between" align="center" mb="6">
        <Button
          variant="ghost"
          color="gray"
          onClick={() => {
            void navigate(`/consignments/${application.consignmentId}/tasks`)
          }}
        >
          <ArrowLeftIcon /> {t('consignments.detail.backButton')}
        </Button>
        <Badge size="2" color={statusColor} highContrast>
          {application.status}
        </Badge>
      </Flex>

      <Box mb="6">
        <Flex align="center" gap="3" mb="1">
          {application.icon?.startsWith('emoji:') && (
            <span className="text-3xl" role="img" aria-label="task-icon">
              {application.icon.slice('emoji:'.length)}
            </span>
          )}
          <h1 className="text-2xl font-bold text-gray-900">
            {application.title || t('consignments.detail.defaultTitle')}
          </h1>
        </Flex>
        {application.description && (
          <Text size="2" color="gray">
            {application.description}
          </Text>
        )}
      </Box>

      {error && (
        <Callout.Root color="red" mb="6">
          <Callout.Icon>
            <ExclamationTriangleIcon />
          </Callout.Icon>
          <Callout.Text>{error}</Callout.Text>
        </Callout.Root>
      )}

      {success && (
        <Callout.Root color="green" mb="6">
          <Callout.Icon>
            <CheckCircledIcon />
          </Callout.Icon>
          <Callout.Text>{t('consignments.detail.success')}</Callout.Text>
        </Callout.Root>
      )}

      {(application.status === 'APPROVED' || application.status === 'REJECTED') && (
        <Callout.Root color={application.status === 'APPROVED' ? 'green' : 'red'} mb="6">
          <Callout.Icon>
            {application.status === 'APPROVED' ? <CheckCircledIcon /> : <ExclamationTriangleIcon />}
          </Callout.Icon>
          <Callout.Text>
            {t('consignments.detail.statusCallout', {
              status: t(`common.status.${application.status.toLowerCase()}`, {
                defaultValue: application.status.toLowerCase().replace(/_/g, ' '),
              }),
            })}
          </Callout.Text>
        </Callout.Root>
      )}

      <div className="space-y-6">
        {/* Main Column */}
        <div className="space-y-6">
          {/* Review form is now at the very top of the page */}
          <Box className="bg-white rounded-lg p-5 border border-gray-200">
            <Flex justify="between" align="center" mb="3">
              <Text
                size="2"
                weight="bold"
                color="gray"
                as="div"
                className="uppercase tracking-wider flex items-center gap-2"
              >
                <InfoCircledIcon />
                {t('consignments.detail.section.review')}
              </Text>
              {application.certificateTemplateId && canReview && (
                <Button
                  variant="soft"
                  size="2"
                  onClick={handleGenerateCertificate}
                  disabled={certificate.loading || !isCertificateDataReady}
                >
                  {certificate.loading ? <Spinner size="1" /> : <FileTextIcon />}
                  {t('consignments.detail.button.generateCertificate')}
                </Button>
              )}
            </Flex>
            {application.certificateTemplateId && canReview && !isCertificateDataReady && (
              <Text size="1" color="gray" mb="3" as="div" style={{ textAlign: 'right' }}>
                {t('consignments.detail.certificate.missingFields')}
              </Text>
            )}
            {certificate.error && (
              <Callout.Root color="red" mb="3">
                <Callout.Icon>
                  <ExclamationTriangleIcon />
                </Callout.Icon>
                <Callout.Text>{t('consignments.detail.certificate.generateFailed')}</Callout.Text>
              </Callout.Root>
            )}
            {isActionable && agencyFormConfig ? (
              <form
                onSubmit={(event) => {
                  void handleSubmit(event)
                }}
                noValidate
              >
                <JsonForms
                  schema={agencyFormConfig.schema}
                  uischema={agencyFormConfig.uiSchema}
                  data={agencyFormData}
                  renderers={radixRenderers}
                  validationMode={showErrors ? 'ValidateAndShow' : 'ValidateAndHide'}
                  onChange={({ data, errors }: { data: Record<string, unknown>; errors?: unknown[] }) => {
                    setAgencyFormData(data)
                    setFormErrors(errors || [])
                  }}
                  ajv={ajvInstance}
                />
                <Flex justify="end" gap="3" mt="6">
                  <Button
                    variant="soft"
                    color="gray"
                    onClick={() => {
                      void navigate('/consignments')
                    }}
                    disabled={isSubmitting}
                    type="button"
                  >
                    {t('consignments.detail.button.cancel')}
                  </Button>
                  <Button type="submit" disabled={isSubmitting}>
                    {isSubmitting ? <Spinner size="1" /> : null}
                    {t('consignments.detail.button.submitReview')}
                  </Button>
                </Flex>
              </form>
            ) : application.status === 'PENDING' && !canReview ? (
              <Text size="2" color="gray" className="italic">
                {t('consignments.detail.empty.noReviewPermission')}
              </Text>
            ) : agencyFormConfig ? (
              <JsonForms
                schema={agencyFormConfig.schema}
                uischema={agencyFormConfig.uiSchema}
                data={agencyFormData}
                renderers={radixRenderers}
                readonly
                ajv={ajvInstance}
              />
            ) : null}
          </Box>

          <Card size="2">
            <Text size="2" weight="bold" color="gray" mb="3" as="div" className="uppercase tracking-wider">
              {t('consignments.detail.section.applicationDetails')}
            </Text>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mt-4">
              <Box>
                <Text size="1" color="gray" as="div" mb="1">
                  {t('consignments.detail.field.consignmentId')}
                </Text>
                <Text size="2" weight="medium" className="break-all font-mono">
                  {application.consignmentId}
                </Text>
              </Box>
              <Box>
                <Text size="1" color="gray" as="div" mb="1">
                  {t('consignments.detail.field.status')}
                </Text>
                <Badge size="2" color={statusColor}>
                  {application.status}
                </Badge>
              </Box>
              <Box>
                <Text size="1" color="gray" as="div" mb="1">
                  {t('consignments.detail.field.submittedOn')}
                </Text>
                <Text size="2" weight="medium">
                  {(() => {
                    const date = new Date(application.createdAt)
                    const resolvedLang = i18n.resolvedLanguage || undefined
                    return t('common.dateTimeAt', {
                      date: date.toLocaleDateString(resolvedLang, {
                        month: 'long',
                        day: 'numeric',
                        year: 'numeric',
                      }),
                      time: date.toLocaleTimeString(resolvedLang, {
                        hour: '2-digit',
                        minute: '2-digit',
                        hour12: true,
                      }),
                    })
                  })()}
                </Text>
              </Box>
            </div>
          </Card>

          <Box className="bg-white rounded-lg p-5 border border-gray-200">
            <Text
              size="2"
              weight="bold"
              color="gray"
              mb="3"
              as="div"
              className="uppercase tracking-wider flex items-center gap-2"
            >
              <InfoCircledIcon />
              {t('consignments.detail.section.submittedInformation')}
            </Text>
            {(() => {
              if (application.dataForm) {
                return (
                  <JsonForms
                    schema={application.dataForm.schema}
                    uischema={application.dataForm.uiSchema}
                    data={application.data}
                    renderers={radixRenderers}
                    readonly={true}
                  />
                )
              }

              if (application.data && Object.keys(application.data).length > 0) {
                return (
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    {Object.entries(application.data).map(([key, value]) => (
                      <Box key={key}>
                        <Text size="1" color="gray" as="div" className="capitalize mb-1">
                          {key.replace(/([A-Z])/g, ' $1').replace(/_/g, ' ')}
                        </Text>
                        <Text size="2" weight="medium">
                          {typeof value === 'object' && value !== null ? JSON.stringify(value) : String(value)}
                        </Text>
                      </Box>
                    ))}
                  </div>
                )
              }

              return (
                <Text size="2" color="gray" className="italic">
                  {t('consignments.detail.empty.noSubmissionData')}
                </Text>
              )
            })()}
          </Box>
        </div>

        {/* Sidebar elements now at the bottom of the main flow */}
        <div className="space-y-6">
          {application.reviewedAt && (
            <Card size="2">
              <Text size="2" weight="bold" color="gray" mb="3" as="div" className="uppercase tracking-wider">
                {t('consignments.detail.section.reviewMetadata')}
              </Text>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mt-3">
                <Box>
                  <Text size="1" color="gray" as="div" mb="1">
                    {t('consignments.detail.field.reviewedOn')}
                  </Text>
                  <Text size="2" weight="medium">
                    {new Date(application.reviewedAt).toLocaleString(i18n.resolvedLanguage || undefined)}
                  </Text>
                </Box>
              </div>
            </Card>
          )}

          {application.reviewerNotes && application.status !== 'PENDING' && (
            <Card size="2">
              <Text size="2" weight="bold" color="gray" mb="3" as="div" className="uppercase tracking-wider">
                {t('consignments.detail.section.reviewerNotes')}
              </Text>
              <Text size="2" className="whitespace-pre-wrap">
                {application.reviewerNotes}
              </Text>
            </Card>
          )}

          {application.feedbackHistory && application.feedbackHistory.length > 0 && (
            <Box className="bg-white rounded-lg p-5 border border-gray-200">
              <Text
                size="2"
                weight="bold"
                color="gray"
                mb="3"
                as="div"
                className="uppercase tracking-wider flex items-center gap-2"
              >
                <ChatBubbleIcon />
                {t('consignments.detail.section.feedbackHistory')}
              </Text>
              <div className="divide-y divide-gray-100">
                {application.feedbackHistory.map((entry) => (
                  <div key={entry.round} className="py-3 first:pt-0 last:pb-0">
                    <Flex justify="between" mb="1">
                      <Text size="1" weight="bold" color="amber">
                        {t('consignments.detail.feedback.round', { round: entry.round })}
                      </Text>
                      <Text size="1" color="gray">
                        {new Date(entry.timestamp).toLocaleString(i18n.resolvedLanguage || undefined)}
                      </Text>
                    </Flex>
                    <Text size="2" className="whitespace-pre-wrap">
                      {entry.content.feedback as string}
                    </Text>
                  </div>
                ))}
              </div>
            </Box>
          )}
        </div>
      </div>

      <CertificatePreviewDialog open={certificate.open} onOpenChange={certificate.setOpen} html={certificate.html} />
    </div>
  )
}
