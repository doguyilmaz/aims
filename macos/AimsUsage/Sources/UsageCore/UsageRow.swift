import Foundation

public struct UsageRow: Decodable, Identifiable, Sendable {
    public let id: String
    public let name: String
    public let plan: String?
    public let detail: String?
    public let windows: [UsageWindow]?
    public let bankedResets: ResetBalance?
    public let observedAt: Date?
    public let source: String?
    public let error: String?
    public let attemptedAt: Date?

    public func isStale(at now: Date) -> Bool {
        guard let observedAt else { return true }
        return now.timeIntervalSince(observedAt) > 7_200 || observedAt > now.addingTimeInterval(60)
    }
}
