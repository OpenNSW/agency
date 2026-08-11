// English translations. Add a new language by creating <lang>.ts and registering it in ../index.ts.
const en = {
  // Auth.tsx, LoginScreen.tsx, UnauthorizedScreen.tsx
  auth: {
    signIn: 'Sign In',
    signOut: 'Sign Out',
    userFallback: 'User',
    login: {
      tagline: 'Sign in to continue to your consignments.',
    },
    unauthorized: {
      title: 'Access Restricted',
      message:
        'Your account does not belong to this agency portal. Please sign in with the correct organisation account.',
    },
  },

  // Sidebar.tsx
  sidebar: {
    nav: {
      consignments: 'Consignments',
    },
    version: {
      label: 'NSW',
    },
    toggle: {
      collapse: 'Collapse',
      expand: 'Expand',
      collapseTitle: 'Collapse sidebar',
      expandTitle: 'Expand sidebar',
    },
  },

  // Layout.tsx, LoginScreen.tsx
  footer: {
    poweredBy: 'Powered by OpenNSW',
  },

  // TopBar.tsx
  topbar: {
    search: {
      placeholder: 'Search...',
    },
  },

  consignments: {
    // ConsignmentListScreen.tsx
    list: {
      title: 'Consignments',
      subtitle: 'Manage and review trader application groups',
      badge: '{{total}} Total Consignments',
      searchPlaceholder: 'Search by Consignment ID...',
      empty: 'No active consignments found.',
      loading: 'Loading consignments...',
      table: {
        id: 'Consignment ID',
        tasks: 'Tasks',
        latestStatus: 'Latest Status',
        lastActivity: 'Last Activity',
      },
    },

    // ConsignmentTasksScreen.tsx
    tasks: {
      title: 'Consignment Tasks',
      consignmentIdLabel: 'ConsignmentId: {{consignmentId}}',
      badge: '{{total}} Total Tasks',
      empty: 'No tasks found for this consignment.',
      loading: 'Loading tasks...',
      error: 'Failed to load tasks. Please try again.',
      retryButton: 'Retry',
      defaultTitle: 'Standard Review',
      backButton: 'Back to Consignments',
      table: {
        task: 'Task',
        category: 'Category',
        status: 'Status',
        lastUpdated: 'Last Updated',
      },
    },

    // ConsignmentDetailScreen.tsx
    detail: {
      loading: 'Loading application details...',
      backButton: 'Back to Tasks',
      backToList: 'Back to List',
      defaultTitle: 'Task Review',
      success: 'Review submitted successfully! Redirecting...',
      statusCallout: 'This application has been {{status}}.',
      notFound: 'Application not found',
      section: {
        review: 'Review',
        applicationDetails: 'Application Details',
        submittedInformation: 'Submitted Information',
        reviewMetadata: 'Review Metadata',
        reviewerNotes: 'Reviewer Notes',
        feedbackHistory: 'Feedback History',
      },
      field: {
        consignmentId: 'Consignment ID',
        status: 'Status',
        submittedOn: 'Submitted On',
        reviewedOn: 'Reviewed On',
      },
      button: {
        cancel: 'Cancel',
        submitReview: 'Submit Review',
      },
      empty: {
        noSubmissionData: 'No submission data available',
        noReviewPermission: 'You do not have permission to review this application.',
      },
      feedback: {
        round: 'Round {{round}}',
      },
    },
  },

  // ConsignmentListScreen.tsx, ConsignmentTasksScreen.tsx
  common: {
    pagination: {
      info: 'Page {{page}} of {{totalPages}} ({{total}} results)',
    },
    dateTimeAt: '{{date}} at {{time}}',
  },

  // ConsignmentDetailScreen.tsx
  errors: {
    noTaskId: 'No task ID provided',
    loadFailed: 'Failed to load application details',
    dataUnavailable: 'Application data not available',
    validationErrors: 'Please fix validation errors before submitting.',
    submitFailed: 'Failed to submit review',
  },
} as const

export default en
