---
name: kotlin-android
description: Kotlin Android 프로젝트용 CLAUDE.md — Gradle, Compose, 아키텍처 컨벤션
---

# Project Instructions

## Build & Test

```bash
./gradlew assembleDebug
./gradlew test
./gradlew lint
./gradlew detekt
```

## Code Style

- Architecture: MVVM with Clean Architecture layers (presentation/domain/data)
- DI: Hilt (`@HiltViewModel`, `@Inject`, `@Module`)
- Async: Coroutines + Flow (avoid RxJava unless existing code)
- State: `StateFlow` for UI state, `SharedFlow` for one-time events
- Navigation: Jetpack Navigation Component or Compose Navigation

## Jetpack Compose

- State hoisting: state up, events down
- `@Preview` for all composables with representative data
- Avoid business logic in composables — delegate to ViewModel
- Use `LaunchedEffect` for side effects, not `DisposableEffect` unless cleanup needed

## Error Handling

- Use `Result<T>` or sealed class for domain layer results
- Map domain errors to UI states in ViewModel
- Never expose exceptions to UI layer directly

## Testing

```bash
./gradlew testDebugUnitTest          # Unit tests
./gradlew connectedDebugAndroidTest  # Instrumented tests
```

- Unit test ViewModels with `TestCoroutineDispatcher`
- Use Robolectric for lightweight Android tests
- UI tests with Espresso or Compose test APIs

# CUSTOMIZE: minSdk, targetSdk, 네비게이션 라이브러리, 이미지 로딩 (Coil/Glide), 네트워크 (Retrofit/Ktor)

## Project Structure

<!-- 모듈 구조 설명 -->
<!-- 예: :app, :feature:home, :core:network, :core:database -->

## Key Libraries

<!-- 주요 라이브러리 버전 -->
