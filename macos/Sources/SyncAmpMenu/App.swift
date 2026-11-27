import SwiftUI

@main
struct SyncAmpMenuApp: App {
    @State private var model = StatusModel()

    var body: some Scene {
        MenuBarExtra {
            PopoverView(model: model)
        } label: {
            Image(systemName: model.symbol)
        }
        .menuBarExtraStyle(.window)
    }
}
