import Foundation

public struct UsageSnapshot: Decodable, Sendable {
    public let schemaVersion: Int
    public let generatedAt: Date
    public let providers: [UsageProvider]
    public let customResets: [CustomReset]?
    public let customResetsError: String?

    public static func decode(_ data: Data) throws -> Self {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let value = try decoder.singleValueContainer().decode(String.self)
            for fractional in [true, false] {
                let style = Date.ISO8601FormatStyle(includingFractionalSeconds: fractional)
                if let date = try? style.parse(value) { return date }
            }
            throw DecodingError.dataCorrupted(
                .init(codingPath: decoder.codingPath, debugDescription: "Invalid usage timestamp"))
        }
        let snapshot = try decoder.decode(Self.self, from: data)
        guard snapshot.schemaVersion == 1 else { throw UsageError.unsupportedSchema }
        return snapshot
    }
}
