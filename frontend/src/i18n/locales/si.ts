// Sinhala translations. Add a new language by creating <lang>.ts and registering it in ../index.ts.
const si = {
  // Auth.tsx, LoginScreen.tsx, UnauthorizedScreen.tsx
  auth: {
    signIn: 'ඇතුළු වන්න',
    signOut: 'පිටවෙන්න',
    userFallback: 'පරිශීලක',
    login: {
      tagline: 'ඔබගේ භාණ්ඩ තොග වෙත යාමට ඇතුළු වන්න.',
    },
    unauthorized: {
      title: 'ප්‍රවේශය සීමා කර ඇත',
      message: 'ඔබගේ ගිණුම මෙම ආයතන ද්වාරයට අයත් නොවේ. කරුණාකර නිවැරදි ආයතනික ගිණුමෙන් ඇතුළු වන්න.',
    },
  },

  // Sidebar.tsx
  sidebar: {
    nav: {
      consignments: 'භාණ්ඩ තොග',
    },
    version: {
      label: 'NSW',
    },
    toggle: {
      collapse: 'හකුලන්න',
      expand: 'දිගහරින්න',
      collapseTitle: 'පැති තීරුව හකුලන්න',
      expandTitle: 'පැති තීරුව දිගහරින්න',
    },
  },

  // TopBar.tsx
  topbar: {
    search: {
      placeholder: 'සොයන්න...',
    },
  },

  consignments: {
    // ConsignmentListScreen.tsx
    list: {
      title: 'භාණ්ඩ තොග',
      subtitle: 'වෙළඳ අයදුම්පත් කණ්ඩායම් කළමනාකරණය සහ සමාලෝචනය කරන්න',
      badge: 'මුළු භාණ්ඩ තොග {{total}}',
      searchPlaceholder: 'භාණ්ඩ තොග හැඳුනුම්පතෙන් සොයන්න...',
      empty: 'සක්‍රිය භාණ්ඩ තොග කිසිවක් හමු නොවීය.',
      loading: 'භාණ්ඩ තොග පූරණය වෙමින් පවතී...',
      table: {
        id: 'භාණ්ඩ තොග හැඳුනුම්පත',
        tasks: 'කාර්යයන්',
        latestStatus: 'නවතම තත්ත්වය',
        lastActivity: 'අවසාන ක්‍රියාකාරකම',
      },
    },

    // ConsignmentTasksScreen.tsx
    tasks: {
      title: 'භාණ්ඩ තොග කාර්යයන්',
      consignmentIdLabel: 'භාණ්ඩ තොග හැඳුනුම්පත: {{consignmentId}}',
      badge: 'මුළු කාර්යයන් {{total}}',
      empty: 'මෙම භාණ්ඩ තොගය සඳහා කාර්යයන් කිසිවක් හමු නොවීය.',
      loading: 'කාර්යයන් පූරණය වෙමින් පවතී...',
      defaultTitle: 'සම්මත සමාලෝචනය',
      backButton: 'භාණ්ඩ තොග වෙත ආපසු',
      table: {
        task: 'කාර්යය',
        category: 'ප්‍රවර්ගය',
        status: 'තත්ත්වය',
        lastUpdated: 'අවසන් වරට යාවත්කාලීන කරන ලද්දේ',
      },
    },

    // ConsignmentDetailScreen.tsx
    detail: {
      loading: 'අයදුම්පත් විස්තර පූරණය වෙමින් පවතී...',
      backButton: 'කාර්යයන් වෙත ආපසු',
      backToList: 'ලැයිස්තුවට ආපසු',
      defaultTitle: 'කාර්ය සමාලෝචනය',
      success: 'සමාලෝචනය සාර්ථකව ඉදිරිපත් කරන ලදී! යොමු වෙමින් පවතී...',
      statusCallout: 'මෙම අයදුම්පත {{status}} කර ඇත.',
      notFound: 'අයදුම්පත හමු නොවීය',
      section: {
        review: 'සමාලෝචනය',
        applicationDetails: 'අයදුම්පත් විස්තර',
        submittedInformation: 'ඉදිරිපත් කළ තොරතුරු',
        reviewMetadata: 'සමාලෝචන පාරදත්ත',
        reviewerNotes: 'සමාලෝචක සටහන්',
        feedbackHistory: 'ප්‍රතිපෝෂණ ඉතිහාසය',
      },
      field: {
        consignmentId: 'භාණ්ඩ තොග හැඳුනුම්පත',
        status: 'තත්ත්වය',
        submittedOn: 'ඉදිරිපත් කළ දිනය',
        reviewedOn: 'සමාලෝචනය කළ දිනය',
      },
      button: {
        cancel: 'අවලංගු කරන්න',
        submitReview: 'සමාලෝචනය ඉදිරිපත් කරන්න',
      },
      empty: {
        noSubmissionData: 'ඉදිරිපත් කළ දත්ත නොමැත',
      },
      feedback: {
        round: 'වටය {{round}}',
      },
    },
  },

  // ConsignmentListScreen.tsx, ConsignmentTasksScreen.tsx
  common: {
    status: {
      approved: 'අනුමත',
      rejected: 'ප්රතික්ෂේප',
      pending: 'අත්හිටුවා',
      feedback_requested: 'ප්රතිපෝෂණ ඉල්ලීම්',
    },
    pagination: {
      info: 'පිටුව {{page}} න් {{totalPages}} (ප්රතිඵල {{total}})',
    },
  },

  // ConsignmentDetailScreen.tsx
  errors: {
    noTaskId: 'කාර්ය හැඳුනුම්පතක් සපයා නැත',
    loadFailed: 'අයදුම්පත් විස්තර පූරණය කිරීමට අපොහොසත් විය',
    dataUnavailable: 'අයදුම්පත් දත්ත ලබා ගත නොහැක',
    validationErrors: 'කරුණාකර ඉදිරිපත් කිරීමට පෙර වලංගුකරණ දෝෂ නිවැරදි කරන්න.',
    submitFailed: 'සමාලෝචනය ඉදිරිපත් කිරීමට අපොහොසත් විය',
  },
} as const

export default si
