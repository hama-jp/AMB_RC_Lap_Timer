<!--
PR の運用ルールは docs/development-workflow.md を参照。
1 PR = 1 トピック。仕様変更は docs/*.md の更新を必ず同 PR に含める。
-->

## Summary
<!-- なぜこの変更が必要か / 何を変えたかを 2-5 行で。 -->

## Related issue
<!-- Closes #N で自動クローズ、Refs #N で関連付け。無ければ「なし」。 -->
- Closes #
- Refs #

## Changes
<!-- 変更点の箇条書き。仕様書の更新があれば必ず明記。 -->
- 

## Self review / Test plan
<!-- マージ前に確認したことを箇条書き。CI 整備までの代替。 -->
- [ ] ローカルでビルドが通る
- [ ] 関連テストを追加 / 既存テストが通る
- [ ] 仕様書(`docs/*.md`)を更新した、または更新不要であることを確認
- [ ] スクリーンショット / ログ(必要に応じて)

## Local verification (Windows)
<!--
ローカル Windows 担当が実行したコマンドと結果を記載(docs/development-workflow.md §8.3)。
コードを伴わない PR(docs のみ等)は「N/A」と記載してよい。
-->
```pwsh
# 例:
# cd gateway
# go test -race -count=1 ./...
# go build ./cmd/gateway
```
- [ ] Go ビルド / テストが Windows で通る(該当時)
- [ ] Web ビルド / テストが Windows で通る(該当時)
- [ ] 実 EXE 起動と `/healthz` 疎通(該当時)
- [ ] 実機 AMB 接続テスト(該当時)

## Notes for reviewer
<!-- レビュー観点・既知の制限・フォローアップ Issue の予告など。 -->

## Follow-up issues
<!-- 派生した気づき・未解決事項を新規 Issue として起票し、ここに番号を列挙。 -->
- Refs #
