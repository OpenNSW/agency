import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { Badge, Text, TextField, Spinner, IconButton } from '@radix-ui/themes'
import { MagnifyingGlassIcon, ChevronLeftIcon, ChevronRightIcon, ArchiveIcon } from '@radix-ui/react-icons'
import { useConsignmentList } from './hooks/useConsignmentList'
import { formatDateForTable } from '@/utils/date'

export function ConsignmentListScreen() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchTerm, setSearchTerm] = useState('')
  const { data, status, pagination } = useConsignmentList(searchTerm)

  if (status.loading && pagination.page === 1) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner size="3" />
        <Text size="3" color="gray" className="ml-3">
          {t('consignments.list.loading')}
        </Text>
      </div>
    )
  }

  return (
    <div className="animate-fade-in max-w-6xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-semibold text-gray-900">{t('consignments.list.title')}</h1>
          <p className="text-gray-500 text-sm mt-1">{t('consignments.list.subtitle')}</p>
        </div>
        <div className="flex items-center gap-4">
          <Badge color="blue" variant="soft" size="2">
            {t('consignments.list.badge', { total: pagination.total })}
          </Badge>
        </div>
      </div>

      <div className="space-y-4">
        <div className="flex flex-col md:flex-row gap-4 mb-6">
          <div className="flex-1">
            <TextField.Root
              size="2"
              placeholder={t('consignments.list.searchPlaceholder')}
              value={searchTerm}
              onChange={(e) => {
                setSearchTerm(e.target.value)
              }}
            >
              <TextField.Slot>
                {status.loading && searchTerm !== '' ? (
                  <Spinner size="1" />
                ) : (
                  <MagnifyingGlassIcon height="16" width="16" />
                )}
              </TextField.Slot>
            </TextField.Root>
          </div>
        </div>

        <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden relative min-h-[400px]">
          {status.loading && (
            <div className="absolute inset-0 bg-white/50 backdrop-blur-[1px] z-10 flex items-center justify-center">
              <div className="flex flex-col items-center gap-2">
                <Spinner size="3" />
                <Text size="2" color="gray">
                  {t('consignments.list.loading')}
                </Text>
              </div>
            </div>
          )}

          {data.length === 0 && !status.loading ? (
            <div className="p-12 text-center">
              <div className="bg-white w-16 h-16 rounded-full flex items-center justify-center mx-auto mb-4 shadow-sm border border-gray-100">
                <ArchiveIcon className="w-8 h-8 text-gray-300" />
              </div>
              <Text size="3" color="gray" weight="medium">
                {t('consignments.list.empty')}
              </Text>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="bg-gray-50/50 border-b border-gray-200 text-left">
                    <th className="px-6 py-4 text-xs font-semibold text-gray-500 uppercase tracking-wider">
                      {t('consignments.list.table.consignment')}
                    </th>
                    <th className="px-6 py-4 text-xs font-semibold text-gray-500 uppercase tracking-wider">
                      {t('consignments.list.table.companyName')}
                    </th>
                    <th className="px-6 py-4 text-xs font-semibold text-gray-500 uppercase tracking-wider text-center">
                      {t('consignments.list.table.tasks')}
                    </th>
                    <th className="px-6 py-4 text-xs font-semibold text-gray-500 uppercase tracking-wider text-center">
                      {t('consignments.list.table.latestStatus')}
                    </th>
                    <th className="px-6 py-4 text-xs font-semibold text-gray-500 uppercase tracking-wider">
                      {t('consignments.list.table.lastActivity')}
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 bg-white">
                  {data.map((consignment) => {
                    const companyName = consignment.traderCompanyName?.trim()
                    return (
                      <tr
                        key={consignment.consignmentId}
                        onClick={() => {
                          void navigate(`/consignments/${consignment.consignmentId}/tasks`)
                        }}
                        className="hover:bg-blue-50/30 cursor-pointer transition-colors group text-sm"
                      >
                        <td className="px-6 py-4">
                          <Text size="2" weight="medium" className="text-blue-600 font-mono break-all hover:underline">
                            {consignment.consignmentId}
                          </Text>
                        </td>
                        <td className="px-6 py-4 text-gray-900 whitespace-nowrap">{companyName || '—'}</td>
                        <td className="px-6 py-4 whitespace-nowrap text-center">{consignment.taskCount}</td>
                        <td className="px-6 py-4 whitespace-nowrap text-center">
                          <Badge
                            size="1"
                            color={
                              consignment.status === 'APPROVED'
                                ? 'green'
                                : consignment.status === 'REJECTED'
                                  ? 'red'
                                  : consignment.status === 'FEEDBACK_REQUESTED'
                                    ? 'amber'
                                    : 'blue'
                            }
                            variant="surface"
                          >
                            {t(`common.status.${consignment.status.toLowerCase()}`, {
                              defaultValue: consignment.status,
                            })}
                          </Badge>
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap text-gray-600">
                          {formatDateForTable(consignment.updatedAt)}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>

        {pagination.totalPages > 1 && (
          <div className="flex items-center justify-between pt-2">
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
    </div>
  )
}
