ALTER TABLE Subscription ADD COLUMN trialDays INTEGER NOT NULL DEFAULT 0;
ALTER TABLE Company ADD COLUMN subscriptionStartedAt DATETIME;
ALTER TABLE Company ADD COLUMN subscriptionEndsAt DATETIME;

UPDATE Company
SET subscriptionStartedAt = (
        SELECT startDate FROM Subscription WHERE Subscription.id = Company.subscriptionId
    ),
    subscriptionEndsAt = (
        SELECT endDate FROM Subscription WHERE Subscription.id = Company.subscriptionId
    )
WHERE subscriptionId IS NOT NULL;

INSERT INTO Subscription (
    id,
    planName,
    planType,
    trialDays,
    maxUsers,
    maxAircraft,
    maxFlightsPerMonth,
    maxStorageGB,
    price,
    currency,
    startDate,
    endDate,
    isActive,
    autoRenew,
    createdAt,
    updatedAt
)
SELECT
    'free-trial-30-days',
    'Free Trial',
    'trial',
    30,
    3,
    1,
    50,
    2,
    0,
    'USD',
    CURRENT_TIMESTAMP,
    datetime('now', '+30 days'),
    1,
    0,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
WHERE NOT EXISTS (
    SELECT 1 FROM Subscription WHERE planType = 'trial'
);
