/**
 * Helper function to get the email subject for trial expiration notifications based on days remaining
 * @param daysRemaining Number of days remaining in the trial
 * @returns Localized email subject in Brazilian Portuguese
 */
export function getEmailSubjectForDaysRemaining(daysRemaining: number): string {
  // Map of days to subject templates
  const subjectTemplates = {
    1: "Último dia do seu período de teste Strait",
    default_even: `${daysRemaining} dias restantes no seu período de teste Strait`,
    default_odd: `Seu período de teste Strait termina em ${daysRemaining} dias`,
  };

  // Special case for 1 day
  if (daysRemaining === 1) {
    return subjectTemplates[1];
  }

  // For all other days, use even/odd templates
  return daysRemaining % 2 === 0
    ? subjectTemplates.default_even
    : subjectTemplates.default_odd;
}
