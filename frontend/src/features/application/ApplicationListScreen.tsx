import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router-dom'
import { Badge, Text, Spinner, IconButton, Button, Flex } from '@radix-ui/themes'
import { ChevronLeftIcon, ChevronRightIcon, ArrowLeftIcon, ArchiveIcon } from '@radix-ui/react-icons'
import { formatDateForTable } from '@/utils/date'
import { useApplicationList } from './hooks/useApplicationList'

export function ApplicationListScreen() {
  const { consignmentId } = useParams<{ consignmentId: string }>()
  return <ApplicationListContent key={consignmentId} consignmentId={consignmentId} />
}

function ApplicationListContent({ consignmentId }: { consignmentId: string | undefined }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data, status, pagination, refetch } = useApplicationList(consignmentId)

  if (status.loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner size="3" />
        <Text size="3" color="gray" className="ml-3">
          {t('consignments.tasks.loading')}
        </Text>
      </div>
    )
  }

  return (
    <div className="animate-fade-in max-w-6xl mx-auto">
      <div className="mb-6">
        <Button
          variant="ghost"
          onClick={() => {
            void navigate('/consignments')
          }}
          className="mb-4 -ml-2 text-gray-600 hover:text-blue-600"
        >
          <ArrowLeftIcon /> {t('consignments.tasks.backButton')}
        </Button>
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-semibold text-gray-900">{t('consignments.tasks.title')}</h1>
            <Text size="2" color="gray" className="font-mono">
              {t('consignments.tasks.consignmentIdLabel', { consignmentId: consignmentId ?? '' })}
            </Text>
          </div>
          <Badge color="blue" variant="soft" size="2">
            {t('consignments.tasks.badge', { total: pagination.total })}
          </Badge>
        </div>
      </div>

      <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
        {status.error ? (
          <div className="p-12 text-center">
            <Text size="3" color="red" weight="medium" className="block mb-4">
              {t('consignments.tasks.error')}
            </Text>
            <Button variant="soft" color="red" onClick={refetch}>
              {t('consignments.tasks.retryButton')}
            </Button>
          </div>
        ) : pagination.total === 0 ? (
          <div className="p-12 text-center">
            <div className="bg-white w-16 h-16 rounded-full flex items-center justify-center mx-auto mb-4 shadow-sm border border-gray-100">
              <ArchiveIcon className="w-8 h-8 text-gray-300" />
            </div>
            <Text size="3" color="gray" weight="medium">
              {t('consignments.tasks.empty')}
            </Text>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="bg-gray-50/50 border-b border-gray-200 text-left">
                  <th className="px-6 py-4 text-xs font-semibold text-gray-500 uppercase tracking-wider">
                    {t('consignments.tasks.table.task')}
                  </th>
                  <th className="px-6 py-4 text-xs font-semibold text-gray-500 uppercase tracking-wider">
                    {t('consignments.tasks.table.category')}
                  </th>
                  <th className="px-6 py-4 text-xs font-semibold text-gray-500 uppercase tracking-wider">
                    {t('consignments.tasks.table.status')}
                  </th>
                  <th className="px-6 py-4 text-xs font-semibold text-gray-500 uppercase tracking-wider">
                    {t('consignments.tasks.table.claimedBy')}
                  </th>
                  <th className="px-6 py-4 text-xs font-semibold text-gray-500 uppercase tracking-wider">
                    {t('consignments.tasks.table.lastUpdated')}
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 bg-white">
                {data.map((app) => (
                  <tr
                    key={app.taskId}
                    onClick={() => {
                      void navigate(`/consignments/${app.consignmentId}?taskId=${app.taskId}`)
                    }}
                    className="hover:bg-blue-50/30 cursor-pointer transition-colors group text-sm"
                  >
                    <td className="px-6 py-4 whitespace-nowrap">
                      <Flex align="center" gap="2">
                        {app.icon?.startsWith('emoji:') && (
                          <span className="text-xl" role="img" aria-label="task-icon">
                            {app.icon.slice('emoji:'.length)}
                          </span>
                        )}
                        <Text size="2" weight="bold" className="text-gray-900">
                          {app.title || t('consignments.tasks.defaultTitle')}
                        </Text>
                      </Flex>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      {app.category && (
                        <Text size="1" color="gray" className="uppercase tracking-tight">
                          {app.category}
                        </Text>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <Badge
                        size="1"
                        color={
                          app.status === 'APPROVED'
                            ? 'green'
                            : app.status === 'REJECTED'
                              ? 'red'
                              : app.status === 'FEEDBACK_REQUESTED'
                                ? 'amber'
                                : 'blue'
                        }
                        variant="surface"
                      >
                        {t(`common.status.${app.status.toLowerCase()}`, { defaultValue: app.status })}
                      </Badge>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      {app.claimedByName ? (
                        <Badge size="1" color="gray" variant="soft">
                          {app.claimedByName}
                        </Badge>
                      ) : (
                        <Text size="1" color="gray" className="italic">
                          {t('consignments.tasks.table.unclaimed')}
                        </Text>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-gray-600">{formatDateForTable(app.updatedAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {pagination.totalPages > 1 && (
        <div className="flex items-center justify-between pt-4">
          <Text size="2" color="gray">
            {t('common.pagination.info', {
              page: pagination.page,
              totalPages: pagination.totalPages,
              total: pagination.total,
            })}
          </Text>
          <div className="flex items-center gap-2">
            <IconButton
              size="1"
              variant="soft"
              disabled={pagination.page <= 1}
              onClick={() => pagination.setPage((p) => p - 1)}
            >
              <ChevronLeftIcon />
            </IconButton>
            <IconButton
              size="1"
              variant="soft"
              disabled={pagination.page >= pagination.totalPages}
              onClick={() => pagination.setPage((p) => p + 1)}
            >
              <ChevronRightIcon />
            </IconButton>
          </div>
        </div>
      )}
    </div>
  )
}
