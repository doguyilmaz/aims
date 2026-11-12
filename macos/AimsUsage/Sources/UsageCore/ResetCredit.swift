import Foundation

public struct ResetCredit: Decodable, Identifiable, Sendable {
    public let id: String
    public let kind: String
    public let status: String
    public let title: String?
    public let grantedAt: Date?
    public let expiresAt: Date?
    public let noExpiry: Bool?
}
