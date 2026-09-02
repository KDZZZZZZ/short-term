# D2 日报

> 成员：A

## 遗留问题的回答

昨天的工程基线解决的是"代码怎么协作"，还没解决"前后端按什么内容协作"。今天把公开 API 固定成唯一契约，免得前端和后端各凭记忆实现接口。

## 目标

完成 OpenAPI 契约和 Swagger 文档：拆分 openapi 目录，维护 schema/path，生成统一 bundle，配好 Redocly 和 npm 校验，并且定下契约漂移怎么查。

## 实际进展

auth、users、products、favorites、conversations、trades 这几组资源都整理完了，schema/path、bundle、Redocly 和 npm 校验也配上了。源文件、生成产物和校验命令的职责分开，契约可以反复验。另外把"联系方式只能改不能清空""PENDING 意向期间不许下架""RESERVED/SOLD 内容冻结"这些业务规则直接写进了契约里。

## 遇到的问题与解决

契约没落成一份之前，接口全靠口头和记忆对，很快就打架了。分页参数我在 `products.yaml` 里写的是 `page/page_size`，在 `favorites.yaml` 里写成了 `offset/limit`，B 看文档的时候直接问我到底哪个算数——我自己都没注意抄岔了。后来把 ID、金额、分页、图片、状态、错误码这些统一收进 `components/`，各 path 只引用不重写。

另一个是 `openapi.bundle.json`。有一次我图快直接改了 bundle，源文件忘了同步，lint 照样过，等于埋了个雷。现在规矩是源契约唯一真源，bundle 只由脚本生成，CI 里重新生成再 diff，对不上就红。

## 后续计划

开始做 Platform 公共基础设施，先把 PostgreSQL、配置、日志、认证、错误、gRPC 这几块的最小边界确认下来。
